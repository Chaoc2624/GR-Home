//go:build darwin

package platform

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// darwinHost is the macOS implementation of Host.
//
// Volumes are the mounted file systems below /Volumes; the system volume
// itself is never listed (it is filtered out by device number, so a
// /Volumes entry that happens to point at the system is skipped too).
type darwinHost struct{}

// New returns the macOS host.
func New() Host { return darwinHost{} }

func (darwinHost) Kind() string { return "macos" }

// ProtectedPaths is empty: Volumes never reports the system volume, and
// the server adds the data directory itself.
func (darwinHost) ProtectedPaths() []string { return nil }

// diskutilTimeout bounds every diskutil call so a stuck disk cannot hang
// the API.
const diskutilTimeout = 10 * time.Second

func runDiskutil(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diskutilTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "diskutil", args...).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// plistPairs parses the flat top-level <dict> of a plist document into
// string values ("true"/"false" for booleans). Nested containers are
// skipped.
func plistPairs(data []byte) map[string]string {
	out := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	var key string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "dict", "array":
				depth++
			case "key":
				if depth == 1 {
					var k string
					if err := dec.DecodeElement(&k, &t); err == nil {
						key = k
					}
				}
			case "string", "integer", "real":
				if depth == 1 && key != "" {
					var v string
					if err := dec.DecodeElement(&v, &t); err == nil {
						out[key] = v
					}
					key = ""
				} else {
					_ = dec.Skip()
				}
			case "true", "false":
				if depth == 1 && key != "" {
					out[key] = t.Name.Local
					key = ""
				}
			default:
				if depth >= 1 && key != "" {
					_ = dec.Skip()
					key = ""
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "dict", "array":
				depth--
			}
		}
	}
	return out
}

// volumeInfo holds what Volumes needs from `diskutil info -plist`.
type volumeInfo struct {
	name      string
	fs        string
	bus       string
	removable bool
}

func probeVolume(path string) (volumeInfo, bool) {
	out, err := runDiskutil("info", "-plist", path)
	if err != nil {
		return volumeInfo{}, false
	}
	p := plistPairs(out)
	vi := volumeInfo{
		name: p["VolumeName"],
		fs:   friendlyFS(p["FilesystemType"]),
		bus:  friendlyBus(p["BusProtocol"]),
	}
	vi.removable = p["Ejectable"] == "true" || p["RemovableMedia"] == "true"
	return vi, true
}

func friendlyFS(fs string) string {
	switch strings.ToLower(fs) {
	case "msdos":
		return "FAT32"
	case "exfat":
		return "exFAT"
	case "apfs":
		return "APFS"
	case "hfs", "hfs+":
		return "HFS+"
	case "ntfs":
		return "NTFS"
	}
	return fs
}

func friendlyBus(bus string) string {
	switch bus {
	case "SecureDigital":
		return "SD"
	case "USB":
		return "USB"
	}
	return bus
}

// sameDevice reports whether path lives on the same device as "/".
func sameDevice(path string) bool {
	var a, b syscall.Stat_t
	if err := syscall.Stat(path, &a); err != nil {
		return false
	}
	if err := syscall.Stat("/", &b); err != nil {
		return false
	}
	return a.Dev == b.Dev
}

func (darwinHost) Volumes(all bool) ([]Volume, error) {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil, fmt.Errorf("read /Volumes: %w", err)
	}
	out := []Volume{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join("/Volumes", e.Name())
		if sameDevice(root) {
			continue // the system volume, never offered
		}
		vi, ok := probeVolume(root)
		if !ok {
			continue // not ready: empty card reader, failing disk, ...
		}
		if !all && !vi.removable {
			continue
		}
		label := vi.name
		if label == "" {
			label = e.Name()
		}
		total, free := diskUsage(root)
		out = append(out, Volume{
			ID:        e.Name(),
			Root:      root,
			Label:     label,
			FS:        vi.fs,
			Total:     total,
			Free:      free,
			Removable: vi.removable,
			Bus:       vi.bus,
		})
	}
	return out, nil
}

func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func (darwinHost) PickDirectory(title string) (string, error) {
	if title == "" {
		title = "Choose a folder"
	}
	script := "POSIX path of (choose folder with prompt " + appleScriptString(title) + ")"
	cmd := exec.Command("osascript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "User canceled") {
			return "", nil
		}
		return "", fmt.Errorf("folder picker: %w", err)
	}
	p := strings.TrimSpace(stdout.String())
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p, nil
}

func (darwinHost) Reveal(path string, isDir bool) error {
	args := []string{"-R", path}
	if isDir {
		args = []string{path}
	}
	cmd := exec.Command("open", args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// Eject unmounts and ejects the volume. Like the Windows implementation it
// never forces anything: when files are still open diskutil fails and the
// volume stays mounted.
func (darwinHost) Eject(vol Volume) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "diskutil", "eject", vol.Root).CombinedOutput()
	if err != nil {
		return fmt.Errorf("diskutil eject: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// OpenURL shows a web address in the default browser.
func (darwinHost) OpenURL(address string) error {
	address, err := BrowserURL(address)
	if err != nil {
		return err
	}
	cmd := exec.Command("open", address)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// Proxy chooses the proxy for an outgoing request: the usual environment
// variables first, then the macOS system proxy settings (read on every
// call, so a change takes effect without a restart).
func (darwinHost) Proxy(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	return systemProxy(req.URL), nil
}

func systemProxy(u *url.URL) *url.URL {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "scutil", "--proxy").Output()
	if err != nil {
		return nil
	}
	vals := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	var hostKey, portKey string
	if u.Scheme == "https" && vals["HTTPSEnable"] == "1" {
		hostKey, portKey = "HTTPSProxy", "HTTPSPort"
	} else if vals["HTTPEnable"] == "1" {
		hostKey, portKey = "HTTPProxy", "HTTPPort"
	} else {
		return nil
	}
	host := vals[hostKey]
	if host == "" {
		return nil
	}
	port := vals[portKey]
	if port == "" {
		port = "8080"
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}
}
