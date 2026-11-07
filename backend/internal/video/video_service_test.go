package video

import (
	"context"
	"errors"
	"feedvid/internal/apierror"
	"testing"
)

func TestPublishValidatesBeforeDatabaseAccess(t *testing.T) {
	service := NewVideoService(nil, nil)
	for _, v := range []*Video{nil, {Title: " ", PlayURL: "video", CoverURL: "cover"}, {Title: "title", PlayURL: " ", CoverURL: "cover"}, {Title: "title", PlayURL: "video", CoverURL: " "}} {
		if err := service.Publish(context.Background(), v); !errors.Is(err, apierror.ErrValidation) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}
