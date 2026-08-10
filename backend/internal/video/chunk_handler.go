package video

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"feedvid/internal/midware/jwt"
	rediscache "feedvid/internal/midware/redis"

	"github.com/gin-gonic/gin"
)

// @TODO: handler 部分 杂合了 service  耦合度高 同时 可以提出一些公共工具
const sessionTTL = 24 * time.Hour

var errChunkCacheUnavailable = errors.New("chunk upload requires redis")

type ChunkUploadHandler struct {
	cache *rediscache.Client
}

func NewChunkUploadHandler(cache *rediscache.Client) *ChunkUploadHandler {
	return &ChunkUploadHandler{cache: cache}
}

// 记录 “上传”动作
func (h *ChunkUploadHandler) sessionKey(uploadID string) string {
	return h.cache.Key("chunk_upload:%s", uploadID)
}

// 记录  谁传 + 传的啥    对应的value 是 上传ID
func (h *ChunkUploadHandler) hashKey(accountID uint, fileHash string) string {
	return h.cache.Key("chunk_upload_hash:%d:%s", accountID, fileHash)
}

// 从redis 中 用 key 获取 session
func (h *ChunkUploadHandler) getSession(ctx *gin.Context, uploadID string) (*ChunkUploadSession, error) {
	if h.cache == nil {
		return nil, errChunkCacheUnavailable
	}
	b, err := h.cache.GetBytes(ctx.Request.Context(), h.sessionKey(uploadID)) // byte类型 -----  存入的时候转换成json了

	if err != nil {
		return nil, fmt.Errorf("upload session not found")
	}
	var s ChunkUploadSession
	if err := json.Unmarshal(b, &s); err != nil { // json读取 byte
		return nil, fmt.Errorf("invalid session data")
	}
	return &s, nil
}

func (h *ChunkUploadHandler) saveSession(ctx *gin.Context, s *ChunkUploadSession) error {
	if h.cache == nil {
		return errChunkCacheUnavailable
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return h.cache.SetBytes(ctx.Request.Context(), h.sessionKey(s.UploadID), b, sessionTTL)
}

// 存的是 hashkey  ----  uploadID
func (h *ChunkUploadHandler) InitChunkUpload(c *gin.Context) {
	if h.cache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errChunkCacheUnavailable.Error()})
		return
	}

	var req InitChunkUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accountID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	const maxSize = 200 << 20
	if req.FileSize > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file size exceeds 200MB limit"})
		return
	}

	// Check for existing session (resume)
	hashKey := h.hashKey(accountID, req.FileHash)
	// hashKey 存的是 上传ID
	existingID, err := h.cache.GetBytes(c.Request.Context(), hashKey)
	if err == nil && len(existingID) > 0 {
		session, sessErr := h.getSession(c, string(existingID))
		if sessErr == nil {
			// Refresh TTL on resume
			_ = h.cache.SetBytes(c.Request.Context(), hashKey, existingID, sessionTTL)
			_ = h.saveSession(c, session)
			c.JSON(http.StatusOK, gin.H{
				"upload_id":       session.UploadID,
				"uploaded_chunks": session.UploadedChunks(),
			})
			return
		}
	}
	// 没有 existingID 的情况
	id, _ := randHex(16)
	uploadID := id + fmt.Sprintf("%d", time.Now().UnixNano())
	session := &ChunkUploadSession{
		UploadID:     uploadID,
		AccountID:    accountID,
		Filename:     req.Filename,
		FileSize:     req.FileSize,
		ChunkSize:    req.ChunkSize,
		TotalChunks:  req.TotalChunks,
		FileHash:     req.FileHash,
		UploadedBits: make([]bool, req.TotalChunks),
	}

	if err := h.saveSession(c, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	if err := h.cache.SetBytes(c.Request.Context(), hashKey, []byte(uploadID), sessionTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_id":       uploadID,
		"uploaded_chunks": []int{},
	})
}

func (h *ChunkUploadHandler) UploadChunk(c *gin.Context) {
	if h.cache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errChunkCacheUnavailable.Error()})
		return
	}

	var req UploadChunkRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.getSession(c, req.UploadID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	accountID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if session.AccountID != accountID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if req.ChunkIndex < 0 || req.ChunkIndex >= session.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk_index"})
		return
	}
	// 如果这个切片已经上传过了，直接返回
	if session.UploadedBits[req.ChunkIndex] {
		c.JSON(http.StatusOK, gin.H{"chunk_index": req.ChunkIndex})
		return
	}
	//  前端固定文件名 "file"
	f, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
		return
	}

	chunkFile, err := f.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read chunk"})
		return
	}
	defer chunkFile.Close()

	//构建新的hash
	hash := md5.New()
	if _, err := io.Copy(hash, chunkFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash chunk"})
		return
	}
	actualHash := fmt.Sprintf("%x", hash.Sum(nil))

	// 检验请求的hash和 传递的hash是否一致
	if actualHash != req.ChunkHash {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chunk hash mismatch", "expected": req.ChunkHash, "actual": actualHash})
		return
	}

	// 创建文件夹路径
	tmpDir := filepath.Join(".run", "uploads", "tmp", req.UploadID)
	// 创建这个路径下的文件夹
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp dir"})
		return
	}

	// 创建 切片存放的路径
	chunkPath := filepath.Join(tmpDir, fmt.Sprintf("%d", req.ChunkIndex))
	if _, seekErr := chunkFile.Seek(0, io.SeekStart); seekErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read chunk"})
		return
	}
	// 在切片路径下创建文件  《----  mkdirALL 先创建文件夹
	dst, err := os.Create(chunkPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save chunk"})
		return
	}
	defer dst.Close()

	// 将切片写入文件
	if _, err := io.Copy(dst, chunkFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save chunk"})
		return
	}

	session.UploadedBits[req.ChunkIndex] = true
	if err := h.saveSession(c, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"chunk_index": req.ChunkIndex})
}

func (h *ChunkUploadHandler) VerifyChunk(c *gin.Context, req UploadChunkRequest) (bool, error) {
	if h.cache == nil {
		return false, errors.New("redis client not initialized")
	}
	// 将 body里传的chunkhash 和 传的chunk 手动计算后的 hash 对比 返回 是否一致

	// 获取传来的 chunk

	f, err := c.FormFile("file")
	if err != nil {
		return false, errors.New("missing file")
	}

	chunkFile, err := f.Open()
	if err != nil {
		return false, errors.New("failed to read chunk")
	}
	defer chunkFile.Close()

	// 创建hash容器

	hash := md5.New()

	if _, err := io.Copy(hash, chunkFile); err != nil {
		return false, errors.New("failed to hash chunk")
	}
	actualHash := fmt.Sprintf("%x", hash.Sum(nil))
	return actualHash == req.ChunkHash, nil
}

func (h *ChunkUploadHandler) ChunkStatus(c *gin.Context) {
	if h.cache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errChunkCacheUnavailable.Error()})
		return
	}

	var req ChunkStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.getSession(c, req.UploadID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	accountID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if session.AccountID != accountID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_id":       session.UploadID,
		"uploaded_chunks": session.UploadedChunks(),
		"total_chunks":    session.TotalChunks,
	})
}

func (h *ChunkUploadHandler) CompleteChunkUpload(c *gin.Context) {
	if h.cache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errChunkCacheUnavailable.Error()})
		return
	}

	var req CompleteChunkUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.getSession(c, req.UploadID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	accountID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if session.AccountID != accountID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	//超过5个切片未上传，则认为上传失败
	if !session.IsComplete() {
		missing := 0
		for _, uploaded := range session.UploadedBits {
			if !uploaded {
				missing++
				if missing > 5 {
					missing = 5
					break
				}
			}
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "not all chunks uploaded",
			"missing":   missing,
			"completed": len(session.UploadedChunks()),
			"total":     session.TotalChunks,
		})
		return
	}

	// 拼接路径
	// .run/uploads/videos/accountID/date/filename.mp4
	date := time.Now().Format("20060102")
	relDir := filepath.Join("videos", fmt.Sprintf("%d", accountID), date)
	root := filepath.Join(".run", "uploads")
	absDir := filepath.Join(root, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create output dir"})
		return
	}

	filename, err := randHex(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate filename"})
		return
	}
	finalPath := filepath.Join(absDir, filename+".mp4")

	finalFile, err := os.Create(finalPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create final file"})
		return
	}
	defer finalFile.Close()

	tmpDir := filepath.Join(".run", "uploads", "tmp", req.UploadID)
	for i := 0; i < session.TotalChunks; i++ {
		chunkPath := filepath.Join(tmpDir, fmt.Sprintf("%d", i))
		cf, err := os.Open(chunkPath)
		if err != nil {
			finalFile.Close()
			os.Remove(finalPath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("chunk %d missing", i)})
			return
		}
		// 把所有切片合并 到一个文件中
		_, err = io.Copy(finalFile, cf)
		cf.Close()
		if err != nil {
			finalFile.Close()
			os.Remove(finalPath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to merge chunks"})
			return
		}
	}
	finalFile.Close()

	// Clean up temp chunks
	_ = os.RemoveAll(tmpDir)

	// Clean up Redis session
	_ = h.cache.Del(c.Request.Context(), h.sessionKey(req.UploadID))
	_ = h.cache.Del(c.Request.Context(), h.hashKey(accountID, session.FileHash))

	urlPath := fmt.Sprintf("/static/videos/%d/%s/%s.mp4", accountID, date, filename)
	playURL := buildAbsoluteURL(c, urlPath)

	c.JSON(http.StatusOK, gin.H{
		"url":      playURL,
		"play_url": playURL,
	})
}
