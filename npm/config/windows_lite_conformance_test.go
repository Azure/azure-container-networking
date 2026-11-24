package npmconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestWindowsLiteConformanceConfig(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("..", "..", ".pipelines", "npm", "windows-npm-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(filepath.Join("..", "..", "test", "npm", "windows-lite", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	reader := viper.New()
	reader.SetConfigType("json")
	if err = reader.ReadConfig(bytes.NewReader(base)); err != nil {
		t.Fatal(err)
	}
	var observed Config
	if err = reader.Unmarshal(&observed); err != nil {
		t.Fatal(err)
	}
	if observed != DefaultConfig {
		t.Fatalf("complete Windows config differs from defaults:\ngot: %+v\nwant: %+v", observed, DefaultConfig)
	}
	if err = reader.MergeConfig(bytes.NewReader(patch)); err != nil {
		t.Fatal(err)
	}
	if err = reader.Unmarshal(&observed); err != nil {
		t.Fatal(err)
	}
	expected := DefaultConfig
	expected.Toggles.EnableV2NPM = true
	expected.Toggles.EnableNPMLite = true
	expected.Toggles.EnableHTTPDebugAPI = false
	expected.Toggles.EnablePprof = false
	if observed != expected {
		t.Fatalf("Lite patch changed unrelated Windows settings:\ngot: %+v\nwant: %+v", observed, expected)
	}
}
