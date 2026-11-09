package core

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDirectFileWritesRequireConfirmedEngineState(t *testing.T) {
	for _, tc := range []struct {
		response string
		want     int
		bad      bool
	}{
		{`{"opt":{"CacheMode":"off"},"activeWriters":0}`, 0, false},
		{`{"opt":{"CacheMode":0},"activeWriters":0}`, 0, false},
		{`{"opt":{"CacheMode":"off"},"activeWriters":2}`, 2, false},
		{`{"opt":{"CacheMode":"full"},"activeWriters":0}`, 0, true},
		{`{"opt":{"CacheMode":"off"}}`, 0, true},
		{`{"opt":{"CacheMode":"off"},"activeWriters":-1}`, 0, true},
	} {
		t.Run(tc.response, func(t *testing.T) {
			rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) {
				var out any
				json.Unmarshal([]byte(tc.response), &out)
				return out, nil
			}}
			c := testConnection()
			c.DisableFileCache = true
			u, used, e := readCacheStats(context.Background(), rpc, c)
			if (e != nil) != tc.bad {
				t.Fatal(e)
			}
			if !tc.bad && (u.Active != tc.want || used != nil) {
				t.Fatalf("unexpected direct-write stats: %+v", u)
			}
		})
	}
}
func TestFileCachePreferenceControlsMountAndPortability(t *testing.T) {
	c := testConnection()
	c.DisableFileCache = true
	v := mountOptions(c, defaultStore().Settings)["vfsOpt"].(map[string]any)
	if v["CacheMode"] != "off" {
		t.Fatal(v)
	}
	if _, ok := v["WriteBack"]; ok {
		t.Fatal("direct writes have delayed cache writes")
	}
	d := portableDefinition(c)
	if !d.connection().DisableFileCache {
		t.Fatal("preference lost during export/import")
	}
}

func TestRequestedFileCacheModeMustMatchEngine(t *testing.T) {
	for _, mode := range []any{"off", 0, "full", 3, nil, "invalid"} {
		for _, disabled := range []bool{false, true} {
			rpc := &fakeRPC{handle: func(context.Context, string, any) (any, error) {
				return map[string]any{"opt": map[string]any{"CacheMode": mode}}, nil
			}}
			c := testConnection()
			c.DisableFileCache = disabled
			err := verifyFileCacheMode(context.Background(), rpc, c)
			match := (disabled && (mode == "off" || mode == 0)) || (!disabled && (mode == "full" || mode == 3))
			if (err == nil) != match {
				t.Fatalf("mode=%v disabled=%t: %v", mode, disabled, err)
			}
		}
	}
}
