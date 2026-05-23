package vfxclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"connectrpc.com/connect"

	storagev1 "github.com/averak/vfx/gen/go/vfx/v1/storage"
)

// QueryFiles lists the player's stored files with their metadata, for diff-sync against local copies via the hash field.
func (c *Client) QueryFiles(ctx context.Context, prefix string) ([]*storagev1.FileMetadata, error) {
	req := connect.NewRequest(&storagev1.QueryFilesRequest{Prefix: prefix})
	c.authorize(req.Header())
	resp, err := c.playerData.QueryFiles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: query files: %w", err)
	}
	return resp.Msg.GetFiles(), nil
}

// WriteFile stores data under filename, hiding the two-step upload: it requests an upload URL, PUTs the bytes directly to the object store, then commits so the gateway records the verified metadata.
func (c *Client) WriteFile(ctx context.Context, filename string, data []byte) error {
	writeReq := connect.NewRequest(&storagev1.WriteFileRequest{
		Filename: filename,
		Size:     uint64(len(data)),
	})
	c.authorize(writeReq.Header())
	writeResp, err := c.playerData.WriteFile(ctx, writeReq)
	if err != nil {
		return fmt.Errorf("vfxclient: write file: %w", err)
	}

	if err := c.putObject(ctx, writeResp.Msg.GetUploadUrl(), writeResp.Msg.GetRequiredHeaders(), data); err != nil {
		return err
	}

	commitReq := connect.NewRequest(&storagev1.CommitFileRequest{Filename: filename})
	c.authorize(commitReq.Header())
	if _, err := c.playerData.CommitFile(ctx, commitReq); err != nil {
		return fmt.Errorf("vfxclient: commit file: %w", err)
	}
	return nil
}

// ReadFile fetches filename's bytes, hiding the URL step: it asks for a download URL and GETs the bytes directly from the object store.
func (c *Client) ReadFile(ctx context.Context, filename string) ([]byte, error) {
	req := connect.NewRequest(&storagev1.ReadFileRequest{Filename: filename})
	c.authorize(req.Header())
	resp, err := c.playerData.ReadFile(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: read file: %w", err)
	}
	return c.getObject(ctx, resp.Msg.GetDownloadUrl())
}

func (c *Client) DeleteFile(ctx context.Context, filename string) error {
	req := connect.NewRequest(&storagev1.DeleteFileRequest{Filename: filename})
	c.authorize(req.Header())
	if _, err := c.playerData.DeleteFile(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: delete file: %w", err)
	}
	return nil
}

// QueryTitleFiles lists operator-published title files carrying all of the given tags (no tags lists everything visible).
func (c *Client) QueryTitleFiles(ctx context.Context, tags []string) ([]*storagev1.FileMetadata, error) {
	req := connect.NewRequest(&storagev1.TitleStorageServiceQueryFilesRequest{Tags: tags})
	c.authorize(req.Header())
	resp, err := c.titleStorage.QueryFiles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: query title files: %w", err)
	}
	return resp.Msg.GetFiles(), nil
}

func (c *Client) ReadTitleFile(ctx context.Context, filename string) ([]byte, error) {
	req := connect.NewRequest(&storagev1.TitleStorageServiceReadFileRequest{Filename: filename})
	c.authorize(req.Header())
	resp, err := c.titleStorage.ReadFile(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: read title file: %w", err)
	}
	return c.getObject(ctx, resp.Msg.GetDownloadUrl())
}

func (c *Client) putObject(ctx context.Context, url string, headers map[string]string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("vfxclient: build upload request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vfxclient: upload: %w", err)
	}
	//nolint:errcheck // Close errors on a drained response body are not actionable.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("vfxclient: upload returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) getObject(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: build download request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: download: %w", err)
	}
	//nolint:errcheck // Close errors on a drained response body are not actionable.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("vfxclient: download returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
