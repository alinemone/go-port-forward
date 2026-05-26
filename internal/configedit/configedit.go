package configedit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
)

// EditorCommand یک *exec.Cmd آماده برای باز کردن مسیر داده‌شده در ادیتور کاربر می‌سازد.
func EditorCommand(path string) (*exec.Cmd, error) {
	editor := pickEditor()
	if editor == "" {
		return nil, fmt.Errorf("no editor found; set $EDITOR or $VISUAL")
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// انتخاب ادیتور: $VISUAL → $EDITOR → پیش‌فرض پلتفرم
func pickEditor() string {
	if e := strings.TrimSpace(os.Getenv("VISUAL")); e != "" {
		return e
	}
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return e
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	for _, candidate := range []string{"vim", "nano", "vi"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// Validate محتوای JSON کانفیگ را تجزیه و اعتبارسنجی می‌کند.
func Validate(data []byte) (*storage.StorageData, error) {
	var sd storage.StorageData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if sd.Services == nil {
		sd.Services = map[string]string{}
	}
	if sd.Groups == nil {
		sd.Groups = map[string][]string{}
	}

	for name, command := range sd.Services {
		if err := manager.ValidateServiceName(name); err != nil {
			return nil, fmt.Errorf("service %q: %v", name, err)
		}
		if err := manager.ValidateCommand(command); err != nil {
			return nil, fmt.Errorf("service %q: %v", name, err)
		}
	}

	for groupName, members := range sd.Groups {
		if err := manager.ValidateServiceName(groupName); err != nil {
			return nil, fmt.Errorf("group %q: %v", groupName, err)
		}
		if _, clash := sd.Services[groupName]; clash {
			return nil, fmt.Errorf("group %q clashes with a service of the same name", groupName)
		}
		for _, member := range members {
			if _, ok := sd.Services[member]; !ok {
				return nil, fmt.Errorf("group %q references unknown service %q", groupName, member)
			}
		}
	}

	return &sd, nil
}
