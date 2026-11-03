package core

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHelpLinksPreserveBrowserDestinations(t *testing.T) {
	expected := map[string]string{
		"winfsp": "https://winfsp.dev/rel/", "webview2": "https://developer.microsoft.com/microsoft-edge/webview2/",
		"oauth-drive": "https://rclone.org/drive/#making-your-own-client-id", "oauth-onedrive": "https://rclone.org/onedrive/#getting-your-own-client-id-and-key", "oauth-sharepoint": "https://rclone.org/onedrive/#getting-your-own-client-id-and-key",
		"oauth-dropbox": "https://rclone.org/dropbox/#get-your-own-dropbox-app-id", "oauth-box": "https://rclone.org/box/#get-your-own-box-app-id", "oauth-pcloud": "https://rclone.org/pcloud/", "oauth-gcs": "https://rclone.org/googlecloudstorage/", "oauth-yandex": "https://yandex.com/dev/id/doc/en/register-client",
	}
	if helpLinksError != nil {
		t.Fatal(helpLinksError)
	}
	if len(helpLinks) != len(expected) {
		t.Fatal("unexpected catalog entries")
	}
	var opened string
	s := &Service{open: func(address string) error { opened = address; return nil }}
	for key, address := range expected {
		opened = ""
		if err := s.Prerequisite(key); err != nil || opened != address {
			t.Fatalf("%s: opened %q, error %v", key, opened, err)
		}
	}
	if s.Prerequisite("unknown") == nil {
		t.Fatal("accepted unknown link")
	}
	marker := errors.New("browser failed")
	s.open = func(string) error { return marker }
	if !errors.Is(s.Prerequisite("winfsp"), marker) {
		t.Fatal("lost browser error")
	}
	s.access.locked = true
	s.open = func(string) error { t.Fatal("opened while locked"); return nil }
	if s.Prerequisite("winfsp") == nil {
		t.Fatal("accepted locked access")
	}
	s.access.locked = false
	s.open = nil
	if s.Prerequisite("winfsp") == nil {
		t.Fatal("accepted missing browser")
	}
}

func TestHelpLinkCatalogRejectsInvalidDestinations(t *testing.T) {
	for _, data := range []string{`{`, `null`, `{}`, `{"x":"file:///tmp/test"}`, `{"x":"https://user:pass@example.test/"}`, `{"x":"https://example.test/a b"}`, `{"x":"https://example.test/\n"}`, `{"x":"https:///path"}`, `{"":"https://example.test/"}`} {
		if _, err := parseHelpLinks([]byte(data)); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
}

func completeHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != ""
}

func TestProductionURLLiteralsUseStructuredResources(t *testing.T) {
	err := filepath.WalkDir("../..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "../.." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || entry.Name() == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(source, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil && completeHTTPURL(value) {
				t.Errorf("%s: store fixed URL in a structured resource: %s", path, value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompleteHTTPURLDistinguishesFragments(t *testing.T) {
	for value, want := range map[string]bool{"https://example.test/help": true, "http://localhost:53682/": true, "https://": false, ".blob.core.windows.net": false, "relative/path": false} {
		if completeHTTPURL(value) != want {
			t.Errorf("incorrect classification: %s", value)
		}
	}
}
