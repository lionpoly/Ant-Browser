package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// Private local file, not encrypted storage. Never log source values.
var proxyImportPreferencesMu sync.Mutex

type ProxyImportSource struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ProxyImportSources struct {
	Sources            []ProxyImportSource `json:"sources"`
	SelectedClashID    string              `json:"selectedClashId"`
	SelectedWebshareID string              `json:"selectedWebshareId"`
}

func (a *App) proxyImportPreferencesPath() string { return a.resolveAppPath("proxy-import.local.json") }

func readProxyImportSources(path string) (ProxyImportSources, error) {
	state := ProxyImportSources{Sources: []ProxyImportSource{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("读取导入信息失败")
	}
	var stored struct {
		ProxyImportSources
		ClashURL       string `json:"clashUrl"`
		WebshareAPIKey string `json:"webshareApiKey"`
	}
	if err := json.Unmarshal(data, &stored); err != nil || strings.TrimSpace(string(data)) == "null" {
		return state, fmt.Errorf("导入信息文件损坏")
	}
	state = stored.ProxyImportSources
	if state.Sources == nil {
		state.Sources = []ProxyImportSource{}
	}
	// Stable IDs make legacy records selectable before the next write.
	if len(state.Sources) == 0 {
		if value := strings.TrimSpace(stored.ClashURL); value != "" {
			state.Sources = append(state.Sources, ProxyImportSource{"legacy-clash", "clash", "Clash 订阅 1", value})
			state.SelectedClashID = "legacy-clash"
		}
		if value := strings.TrimSpace(stored.WebshareAPIKey); value != "" {
			state.Sources = append(state.Sources, ProxyImportSource{"legacy-webshare", "webshare", "Webshare 账号 1", value})
			state.SelectedWebshareID = "legacy-webshare"
		}
	}
	return state, nil
}

func writeProxyImportSources(path string, state ProxyImportSources) error {
	if len(state.Sources) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("清除导入信息失败")
		}
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("编码导入信息失败")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("创建导入信息目录失败")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".proxy-import-*.local.json")
	if err != nil {
		return fmt.Errorf("创建导入信息文件失败")
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("写入导入信息失败")
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("保存导入信息失败")
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("替换导入信息失败")
	}
	return nil
}

func (a *App) BrowserProxyGetImportSources() (ProxyImportSources, error) {
	proxyImportPreferencesMu.Lock()
	defer proxyImportPreferencesMu.Unlock()
	return readProxyImportSources(a.proxyImportPreferencesPath())
}

func saveProxyImportSource(state *ProxyImportSources, id, kind, name, value string) (ProxyImportSource, error) {
	value = strings.TrimSpace(value)
	name = strings.TrimSpace(name)
	if kind != "clash" && kind != "webshare" {
		return ProxyImportSource{}, fmt.Errorf("未知的导入来源类型")
	}
	if value == "" || len(value) > 16384 || len(name) > 300 {
		return ProxyImportSource{}, fmt.Errorf("请填写有效的来源名称及订阅 URL / API key")
	}
	index := -1
	for i, s := range state.Sources {
		if s.ID == id && id != "" {
			if s.Kind != kind {
				return ProxyImportSource{}, fmt.Errorf("不能修改来源类型")
			}
			index = i
		}
		if id == "" && s.Kind == kind && s.Value == value {
			index = i
		}
	}
	if id != "" && index < 0 {
		return ProxyImportSource{}, fmt.Errorf("来源已不存在，请重新选择")
	}
	for i, s := range state.Sources {
		if i != index && s.Kind == kind && s.Value == value {
			return ProxyImportSource{}, fmt.Errorf("该来源已保存，请选择已有来源")
		}
	}
	if index < 0 {
		if len(state.Sources) >= 200 {
			return ProxyImportSource{}, fmt.Errorf("最多保存 200 个导入来源")
		}
		index = len(state.Sources)
		state.Sources = append(state.Sources, ProxyImportSource{ID: uuid.NewString(), Kind: kind})
	}
	if name == "" {
		if kind == "clash" {
			name = "Clash 订阅"
		} else {
			name = "Webshare 账号"
		}
		name = fmt.Sprintf("%s %d", name, index+1)
	}
	source := state.Sources[index]
	source.Name = name
	source.Value = value
	state.Sources[index] = source
	if kind == "clash" {
		state.SelectedClashID = source.ID
	} else {
		state.SelectedWebshareID = source.ID
	}
	return source, nil
}

func (a *App) BrowserProxySaveImportSource(id, kind, name, value string) (ProxyImportSource, error) {
	proxyImportPreferencesMu.Lock()
	defer proxyImportPreferencesMu.Unlock()
	path := a.proxyImportPreferencesPath()
	state, err := readProxyImportSources(path)
	if err != nil {
		return ProxyImportSource{}, err
	}
	source, err := saveProxyImportSource(&state, id, kind, name, value)
	if err != nil {
		return source, err
	}
	return source, writeProxyImportSources(path, state)
}

func (a *App) BrowserProxyDeleteImportSource(id string) error {
	proxyImportPreferencesMu.Lock()
	defer proxyImportPreferencesMu.Unlock()
	path := a.proxyImportPreferencesPath()
	state, err := readProxyImportSources(path)
	if err != nil {
		return err
	}
	kept := make([]ProxyImportSource, 0, len(state.Sources))
	for _, s := range state.Sources {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	state.Sources = kept
	if state.SelectedClashID == id {
		state.SelectedClashID = ""
	}
	if state.SelectedWebshareID == id {
		state.SelectedWebshareID = ""
	}
	return writeProxyImportSources(path, state)
}

// Compatibility with the single-source API used by older frontend bundles.
func (a *App) BrowserProxyGetImportPreferences() (map[string]string, error) {
	state, err := a.BrowserProxyGetImportSources()
	if err != nil {
		return nil, err
	}
	values := map[string]string{"clashUrl": "", "webshareApiKey": ""}
	for _, s := range state.Sources {
		if s.ID == state.SelectedClashID {
			values["clashUrl"] = s.Value
		}
		if s.ID == state.SelectedWebshareID {
			values["webshareApiKey"] = s.Value
		}
	}
	return values, nil
}

func (a *App) BrowserProxySaveImportPreference(field, value string) error {
	kind := "clash"
	if field == "webshareApiKey" {
		kind = "webshare"
	} else if field != "clashUrl" {
		return fmt.Errorf("未知的导入信息类型")
	}
	proxyImportPreferencesMu.Lock()
	defer proxyImportPreferencesMu.Unlock()
	path := a.proxyImportPreferencesPath()
	state, err := readProxyImportSources(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		kept := make([]ProxyImportSource, 0, len(state.Sources))
		for _, s := range state.Sources {
			if s.Kind != kind {
				kept = append(kept, s)
			}
		}
		state.Sources = kept
		if kind == "clash" {
			state.SelectedClashID = ""
		} else {
			state.SelectedWebshareID = ""
		}
	} else if _, err := saveProxyImportSource(&state, "", kind, "", value); err != nil {
		return err
	}
	return writeProxyImportSources(path, state)
}
