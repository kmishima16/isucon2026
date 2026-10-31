package isureal

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

type exportPostResponse struct {
	createPostResponse
	ImagePath string `json:"imagePath"`
}

func exportPostsHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		userID := GetSession(c).UserID
		author, err := getUser(ctx, h.DB, userID)
		if err != nil {
			return err
		}
		// 画像の中身は、この台が持っていれば DB から読まない（image_data_cache.go）。エクスポートは 1 人の全投稿の画像を読み、DB の読み込みの上位だった
		var imageIDs []string
		if err := h.DB.SelectContext(ctx, &imageIDs, `SELECT image_id FROM posts WHERE author_id = $1 AND deleted_at IS NULL`, userID); err != nil {
			return err
		}
		cached := make(map[string][]byte, len(imageIDs))
		cachedIDs := make([]string, 0, len(imageIDs))
		for _, id := range imageIDs {
			if data, ok := h.ImageData.get(id); ok {
				cached[id] = data
				cachedIDs = append(cachedIDs, id)
			}
		}
		var posts []struct {
			Post
			ImageData []byte `db:"image_data"`
		}
		// image_objects は created_at の月で分けてあり、id だけでは全区画を引く。投稿の画像は posted_at で書くので区画を 1 つに絞れる
		if err := h.DB.SelectContext(ctx, &posts, `
   SELECT `+postColumns+`, CASE WHEN i.id = ANY($2) THEN NULL ELSE i.data END AS image_data
   FROM posts p
   JOIN posting_windows w ON w.id = p.posting_window_id
   JOIN image_objects i ON i.id = p.image_id AND i.created_at = p.posted_at
   WHERE p.author_id = $1 AND p.deleted_at IS NULL
   ORDER BY p.posted_at, p.id
  `, userID, cachedIDs); err != nil {
			return err
		}
		for i := range posts {
			if data, ok := cached[posts[i].ImageID]; ok {
				posts[i].ImageData = data
			} else {
				h.ImageData.set(posts[i].ImageID, posts[i].ImageData)
			}
		}

		var body bytes.Buffer
		writer := zip.NewWriter(&body)
		defer writer.Close()
		items := make([]exportPostResponse, 0, len(posts))
		for _, post := range posts {
			imagePath := fmt.Sprintf("images/%06d.jpg", len(items)+1)
			image, err := writer.CreateHeader(&zip.FileHeader{Name: imagePath, Method: zip.Store})
			if err != nil {
				return err
			}
			if _, err := image.Write(post.ImageData); err != nil {
				return err
			}
			items = append(items, exportPostResponse{createPostResponse: newPostResponse(post.Post, author), ImagePath: imagePath})
		}
		manifest, err := writer.Create("posts.json")
		if err != nil {
			return err
		}
		if err := json.NewEncoder(manifest).Encode(items); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
		c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="isureal-posts.zip"`)
		c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
		return c.Blob(http.StatusOK, "application/zip", body.Bytes())
	}
}
