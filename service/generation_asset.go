package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/creation_setting"

	_ "golang.org/x/image/webp"
)

const generationSniffBytes = 512

var (
	ErrGenerationAssetTooLarge = errors.New("generation asset exceeds the file size limit")
	ErrUnsupportedAssetType    = errors.New("unsupported generation asset type")
)

type GenerationAssetSaveRequest struct {
	UserID             int
	JobID              int64
	Role               string
	Kind               string
	Reader             io.Reader
	MaxBytes           int64
	ConsumeReservation bool
}

func GenerationAssetLimit(kind string) int64 {
	setting := creation_setting.GetSetting()
	if kind == model.GenerationKindVideo {
		return int64(setting.MaxVideoMB) * 1024 * 1024
	}
	return int64(setting.MaxImageMB) * 1024 * 1024
}

func SaveGenerationAsset(request GenerationAssetSaveRequest) (*model.GenerationAsset, error) {
	if request.UserID <= 0 || request.Reader == nil {
		return nil, errors.New("invalid generation asset request")
	}
	if request.Role != "input" && request.Role != "output" {
		return nil, errors.New("invalid generation asset role")
	}
	if request.Kind != model.GenerationKindImage && request.Kind != model.GenerationKindVideo {
		return nil, errors.New("invalid generation asset kind")
	}
	if request.MaxBytes <= 0 {
		request.MaxBytes = GenerationAssetLimit(request.Kind)
	}

	root, err := filepath.Abs(creation_setting.AssetRoot())
	if err != nil {
		return nil, fmt.Errorf("resolve generation asset root: %w", err)
	}
	userDir := filepath.Join(root, "user-"+strconv.Itoa(request.UserID))
	if err := os.MkdirAll(userDir, 0o750); err != nil {
		return nil, fmt.Errorf("create generation asset directory: %w", err)
	}

	randomName, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, fmt.Errorf("generate asset name: %w", err)
	}
	temporaryPath := filepath.Join(userDir, ".part-"+randomName)
	temporaryFile, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create generation asset: %w", err)
	}

	removeTemporary := true
	defer func() {
		_ = temporaryFile.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	written, err := io.Copy(temporaryFile, io.LimitReader(request.Reader, request.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("write generation asset: %w", err)
	}
	if written > request.MaxBytes {
		return nil, ErrGenerationAssetTooLarge
	}
	if written == 0 {
		return nil, ErrUnsupportedAssetType
	}
	if err := temporaryFile.Sync(); err != nil {
		return nil, fmt.Errorf("sync generation asset: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return nil, fmt.Errorf("close generation asset: %w", err)
	}

	mimeType, extension, err := inspectGenerationAsset(temporaryPath, request.Kind)
	if err != nil {
		return nil, err
	}
	finalName := randomName + extension
	finalPath := filepath.Join(userDir, finalName)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return nil, fmt.Errorf("finalize generation asset: %w", err)
	}
	removeTemporary = false

	relativePath, err := filepath.Rel(root, finalPath)
	if err != nil {
		_ = os.Remove(finalPath)
		return nil, fmt.Errorf("build generation asset path: %w", err)
	}
	asset := &model.GenerationAsset{
		JobID:        request.JobID,
		UserID:       request.UserID,
		Role:         request.Role,
		RelativePath: filepath.ToSlash(relativePath),
		MimeType:     mimeType,
		SizeBytes:    written,
	}
	if err := model.InsertGenerationAsset(asset, request.ConsumeReservation); err != nil {
		_ = os.Remove(finalPath)
		return nil, err
	}
	return asset, nil
}

func SaveRemoteGenerationAsset(ctx context.Context, rawURL string, request GenerationAssetSaveRequest) (*model.GenerationAsset, error) {
	return SaveRemoteGenerationAssetWithHeaders(ctx, rawURL, nil, request)
}

func SaveRemoteGenerationAssetWithHeaders(ctx context.Context, rawURL string, headers http.Header, request GenerationAssetSaveRequest) (*model.GenerationAsset, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("generation asset URL must use HTTPS")
	}
	if err := ValidateSSRFProtectedFetchURL(parsed.String()); err != nil {
		return nil, fmt.Errorf("generation asset URL blocked: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create generation asset request: %w", err)
	}
	for key, values := range headers {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Cookie") {
			continue
		}
		for _, value := range values {
			httpRequest.Header.Add(key, value)
		}
	}
	response, err := GetSSRFProtectedHTTPClient().Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("download generation asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("generation asset upstream returned status %d", response.StatusCode)
	}
	if request.MaxBytes <= 0 {
		request.MaxBytes = GenerationAssetLimit(request.Kind)
	}
	if response.ContentLength > request.MaxBytes {
		return nil, ErrGenerationAssetTooLarge
	}
	request.Reader = response.Body
	return SaveGenerationAsset(request)
}

func GenerationAssetPath(asset *model.GenerationAsset) (string, error) {
	if asset == nil || strings.TrimSpace(asset.RelativePath) == "" {
		return "", errors.New("invalid generation asset")
	}
	root, err := filepath.Abs(creation_setting.AssetRoot())
	if err != nil {
		return "", fmt.Errorf("resolve generation asset root: %w", err)
	}
	relativePath := filepath.FromSlash(asset.RelativePath)
	if filepath.IsAbs(relativePath) || filepath.VolumeName(relativePath) != "" || strings.HasPrefix(relativePath, string(filepath.Separator)) {
		return "", errors.New("absolute generation asset path is not allowed")
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("generation asset path escapes the storage root")
	}
	absolutePath := filepath.Join(root, cleaned)
	containedPath, err := filepath.Rel(root, absolutePath)
	if err != nil || containedPath == ".." || strings.HasPrefix(containedPath, ".."+string(filepath.Separator)) {
		return "", errors.New("generation asset path escapes the storage root")
	}
	return absolutePath, nil
}

func OpenGenerationAsset(asset *model.GenerationAsset) (*os.File, error) {
	path, err := GenerationAssetPath(asset)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func RemoveGenerationAssetFile(asset *model.GenerationAsset) error {
	path, err := GenerationAssetPath(asset)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func CleanupExpiredGenerationAssets(now int64, limit int) (int, error) {
	assets, err := model.ListExpiredGenerationAssets(now, limit)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for i := range assets {
		asset := &assets[i]
		if err := RemoveGenerationAssetFile(asset); err != nil {
			return deleted, err
		}
		if err := model.DeleteGenerationAssetRecord(asset.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	if err := model.DeleteExpiredEmptyGenerationJobs(now, limit); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func inspectGenerationAsset(path string, kind string) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, generationSniffBytes)
	header, err := reader.Peek(generationSniffBytes)
	if err != nil && !errors.Is(err, bufio.ErrBufferFull) && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	if kind == model.GenerationKindVideo {
		return inspectGenerationVideo(header)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	format, err := decodeGenerationImageFormat(file)
	if err != nil {
		return "", "", ErrUnsupportedAssetType
	}
	switch format {
	case "png":
		return "image/png", ".png", nil
	case "jpeg":
		return "image/jpeg", ".jpg", nil
	case "webp":
		return "image/webp", ".webp", nil
	default:
		return "", "", ErrUnsupportedAssetType
	}
}

func decodeGenerationImageFormat(reader io.Reader) (string, error) {
	_, format, err := image.DecodeConfig(reader)
	return format, err
}

func inspectGenerationVideo(header []byte) (string, string, error) {
	if len(header) >= 12 && bytes.Equal(header[4:8], []byte("ftyp")) {
		brand := string(header[8:12])
		if brand == "qt  " {
			return "video/quicktime", ".mov", nil
		}
		return "video/mp4", ".mp4", nil
	}
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return "video/webm", ".webm", nil
	}
	return "", "", ErrUnsupportedAssetType
}

func GenerationAssetExpiryTime(asset *model.GenerationAsset) time.Time {
	if asset == nil || asset.ExpiresAt <= 0 {
		return time.Time{}
	}
	return time.Unix(asset.ExpiresAt, 0)
}
