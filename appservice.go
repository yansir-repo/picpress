package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"picpress/internal/imagepress"
)

const (
	maxFilesPerAdd = 40
	maxInputBytes  = 300 << 20 // 300 MiB
)

// AppService is bound to the frontend. All image work happens in Go; the
// webview only renders state and talks to this service.
type AppService struct {
	app *application.App

	mu     sync.Mutex
	items  map[string]*sessionItem
	paths  map[string]string // absolute source path -> item id
	order  []string
	seq    int64
	tmpDir string
}

type sessionItem struct {
	info    imagepress.ItemInfo
	encoded string
	result  *imagepress.CompressResult
}

// SaveAllResult reports where a batch export landed.
type SaveAllResult struct {
	Directory string   `json:"directory"`
	Saved     []string `json:"saved"`
	Errors    []string `json:"errors"`
}

// NewAppService creates a session backed by a private temporary directory.
// The application is attached right after application.New so the service
// itself can be registered through the normal options.
func NewAppService() *AppService {
	return &AppService{
		items: map[string]*sessionItem{},
		paths: map[string]string{},
	}
}

func (s *AppService) attach(app *application.App) {
	s.app = app
}

func (s *AppService) workDir() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tmpDir != "" {
		return s.tmpDir, nil
	}
	dir, err := os.MkdirTemp("", "picpress-*")
	if err != nil {
		return "", fmt.Errorf("create temporary directory: %w", err)
	}
	s.tmpDir = dir
	return dir, nil
}

// AddFiles validates and registers the given image paths.
func (s *AppService) AddFiles(paths []string) imagepress.AddFilesResult {
	result := imagepress.AddFilesResult{
		Items:  []imagepress.ItemInfo{},
		Errors: []imagepress.FileError{},
	}
	for _, path := range paths {
		if len(result.Items) >= maxFilesPerAdd {
			result.Errors = append(result.Errors, imagepress.FileError{
				Path:    path,
				Message: fmt.Sprintf("一次最多添加 %d 张图片", maxFilesPerAdd),
			})
			continue
		}
		info, err := s.addFile(path)
		if err != nil {
			result.Errors = append(result.Errors, imagepress.FileError{Path: path, Message: err.Error()})
			continue
		}
		result.Items = append(result.Items, info)
	}
	return result
}

func (s *AppService) addFile(path string) (imagepress.ItemInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return imagepress.ItemInfo{}, err
	}
	if existing, ok := s.lookup(abs); ok {
		return existing, nil
	}

	stat, err := os.Stat(abs)
	if err != nil {
		return imagepress.ItemInfo{}, fmt.Errorf("无法读取文件")
	}
	if stat.IsDir() {
		return imagepress.ItemInfo{}, fmt.Errorf("这是一个文件夹")
	}
	if stat.Size() == 0 {
		return imagepress.ItemInfo{}, fmt.Errorf("文件为空")
	}
	if stat.Size() > maxInputBytes {
		return imagepress.ItemInfo{}, fmt.Errorf("文件超过 300 MB")
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return imagepress.ItemInfo{}, fmt.Errorf("读取失败：%v", err)
	}
	img, format, err := imagepress.Decode(data)
	if err != nil {
		return imagepress.ItemInfo{}, fmt.Errorf("不支持的图片：%v", err)
	}

	info := imagepress.ItemInfo{
		Name:     filepath.Base(abs),
		Path:     abs,
		Format:   format,
		Width:    img.Bounds().Dx(),
		Height:   img.Bounds().Dy(),
		Bytes:    stat.Size(),
		HasAlpha: imagepress.HasAlpha(img),
		Preview:  imagepress.PreviewDataURL(img, 900),
	}
	if format == "gif" {
		info.Warning = "动图只会保留第一帧"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.paths[abs]; ok {
		return s.items[id].info, nil
	}
	s.seq++
	info.ID = fmt.Sprintf("img_%d", s.seq)
	s.items[info.ID] = &sessionItem{info: info}
	s.paths[abs] = info.ID
	s.order = append(s.order, info.ID)
	return info, nil
}

func (s *AppService) lookup(path string) (imagepress.ItemInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.paths[path]
	if !ok {
		return imagepress.ItemInfo{}, false
	}
	return s.items[id].info, true
}

// PickFiles opens the native file picker and adds the selection.
func (s *AppService) PickFiles() (imagepress.AddFilesResult, error) {
	dialog := s.app.Dialog.OpenFile().
		SetTitle("选择图片").
		SetMessage("选择要压缩的图片").
		SetButtonText("添加").
		CanCreateDirectories(true).
		AddFilter("图片文件", "*.jpg;*.jpeg;*.png;*.webp;*.avif;*.heic;*.heif;*.gif")

	if win := s.app.Window.Current(); win != nil {
		dialog.AttachToWindow(win)
	}
	files, err := dialog.PromptForMultipleSelection()
	if err != nil {
		return imagepress.AddFilesResult{}, err
	}
	if len(files) == 0 {
		return imagepress.AddFilesResult{Items: []imagepress.ItemInfo{}, Errors: []imagepress.FileError{}}, nil
	}
	return s.AddFiles(files), nil
}

// Compress encodes the selected items (or the whole session when ids is empty).
func (s *AppService) Compress(ids []string, opts imagepress.Options) []imagepress.CompressResult {
	items := s.selected(ids)
	opts = opts.Normalized()

	results := make([]imagepress.CompressResult, 0, len(items))
	for i, it := range items {
		s.app.Event.Emit("compress:progress", map[string]any{
			"id":    it.info.ID,
			"index": i + 1,
			"total": len(items),
		})
		results = append(results, s.compressOne(it, opts))
	}
	return results
}

func (s *AppService) compressOne(it *sessionItem, opts imagepress.Options) imagepress.CompressResult {
	res := imagepress.CompressResult{
		ID:             it.info.ID,
		OriginalBytes:  it.info.Bytes,
		OriginalWidth:  it.info.Width,
		OriginalHeight: it.info.Height,
	}

	data, err := os.ReadFile(it.info.Path)
	if err != nil {
		res.Error = "读取原图失败"
		return res
	}
	img, _, err := imagepress.Decode(data)
	if err != nil {
		res.Error = "解码失败：" + err.Error()
		return res
	}

	started := time.Now()
	enc, err := imagepress.Compress(img, opts)
	res.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		res.Error = "压缩失败：" + err.Error()
		return res
	}

	dir, err := s.workDir()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	outPath := filepath.Join(dir, it.info.ID+enc.Format.Ext())
	if err := os.WriteFile(outPath, enc.Data, 0o644); err != nil {
		res.Error = "写入临时文件失败"
		return res
	}

	res.Format = string(enc.Format)
	res.Width = enc.Width
	res.Height = enc.Height
	res.Bytes = int64(len(enc.Data))
	res.OutputPath = outPath
	if it.info.Bytes > 0 {
		res.SavedPercent = math.Round((1-float64(len(enc.Data))/float64(it.info.Bytes))*1000) / 10
	}
	if res.SavedPercent < 0 {
		res.Warning = "压缩后反而更大，建议降低质量、调整尺寸或换用 WebP"
	}
	if opts.Format == imagepress.FormatJPEG && it.info.HasAlpha {
		res.Warning = "透明区域已填充为背景色"
	}
	if dec, derr := imagepress.DecodeEncoded(enc.Data); derr == nil {
		res.Preview = imagepress.PreviewDataURL(dec, 900)
	}

	s.mu.Lock()
	if current, ok := s.items[it.info.ID]; ok {
		if current.encoded != "" && current.encoded != outPath {
			_ = os.Remove(current.encoded)
		}
		copy := res
		current.result = &copy
		current.encoded = outPath
	}
	s.mu.Unlock()

	return res
}

// SaveFile asks for a destination and writes one compressed image there.
func (s *AppService) SaveFile(id string) (string, error) {
	it := s.get(id)
	if it == nil || it.result == nil || it.encoded == "" {
		return "", fmt.Errorf("图片尚未压缩")
	}

	ext := imagepress.Format(it.result.Format).Ext()
	base := strings.TrimSuffix(it.info.Name, filepath.Ext(it.info.Name))
	dialog := s.app.Dialog.SaveFile().
		SetMessage("保存压缩后的图片").
		SetButtonText("保存").
		SetDirectory(filepath.Dir(it.info.Path)).
		SetFilename(base+"-compressed"+ext).
		AddFilter(strings.ToUpper(strings.TrimPrefix(ext, "."))+" 图片", "*"+ext).
		AllowsOtherFileTypes(false).
		CanCreateDirectories(true)

	if win := s.app.Window.Current(); win != nil {
		dialog.AttachToWindow(win)
	}
	dest, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if dest == "" {
		return "", nil
	}
	if filepath.Ext(dest) == "" {
		dest += ext
	}

	data, err := os.ReadFile(it.encoded)
	if err != nil {
		return "", fmt.Errorf("读取压缩结果失败")
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("写入失败：%v", err)
	}
	return dest, nil
}

// SaveAll writes every compressed result into a directory chosen by the user.
func (s *AppService) SaveAll(ids []string) (SaveAllResult, error) {
	result := SaveAllResult{Saved: []string{}, Errors: []string{}}
	items := s.selected(ids)

	dialog := s.app.Dialog.OpenFile().
		SetTitle("选择保存文件夹").
		SetMessage("所有图片会保存到这个文件夹").
		SetButtonText("保存到这里").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true)
	if win := s.app.Window.Current(); win != nil {
		dialog.AttachToWindow(win)
	}
	dir, err := dialog.PromptForSingleSelection()
	if err != nil {
		return result, err
	}
	if dir == "" {
		return result, nil
	}
	result.Directory = dir

	for _, it := range items {
		if it.result == nil || it.encoded == "" {
			continue
		}
		ext := imagepress.Format(it.result.Format).Ext()
		base := strings.TrimSuffix(it.info.Name, filepath.Ext(it.info.Name))
		dest := uniquePath(dir, base+"-compressed", ext)
		data, err := os.ReadFile(it.encoded)
		if err != nil {
			result.Errors = append(result.Errors, it.info.Name+": 读取失败")
			continue
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			result.Errors = append(result.Errors, it.info.Name+": 写入失败")
			continue
		}
		result.Saved = append(result.Saved, dest)
	}
	return result, nil
}

// Remove drops one item and its temporary output.
func (s *AppService) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok {
		return false
	}
	if it.encoded != "" {
		_ = os.Remove(it.encoded)
	}
	delete(s.items, id)
	delete(s.paths, it.info.Path)
	for i, existing := range s.order {
		if existing == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return true
}

// Clear empties the session.
func (s *AppService) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.items {
		if it.encoded != "" {
			_ = os.Remove(it.encoded)
		}
	}
	s.items = map[string]*sessionItem{}
	s.paths = map[string]string{}
	s.order = nil
	return true
}

// RevealPath shows a saved file or folder in Finder / Explorer.
func (s *AppService) RevealPath(path string) bool {
	if path == "" {
		return false
	}
	selectFile := true
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		selectFile = false
	}
	return s.app.Env.OpenFileManager(path, selectFile) == nil
}

// Cleanup removes the temporary workspace.
func (s *AppService) Cleanup() {
	s.mu.Lock()
	dir := s.tmpDir
	s.tmpDir = ""
	s.mu.Unlock()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

func (s *AppService) get(id string) *sessionItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := s.items[id]
	if it == nil {
		return nil
	}
	copy := *it
	if it.result != nil {
		res := *it.result
		copy.result = &res
	}
	return &copy
}

func (s *AppService) selected(ids []string) []*sessionItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	items := make([]*sessionItem, 0, len(s.order))
	for _, id := range s.order {
		if len(wanted) > 0 && !wanted[id] {
			continue
		}
		it := s.items[id]
		if it == nil {
			continue
		}
		copy := *it
		items = append(items, &copy)
	}
	return items
}

func uniquePath(dir, base, ext string) string {
	path := filepath.Join(dir, base+ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if i > 500 {
			return path
		}
	}
}
