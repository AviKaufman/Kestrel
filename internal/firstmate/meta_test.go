package firstmate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMetaKeepsLastRepeatedValueAndEmbeddedEquals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.meta")
	content := "window=firstmate:fm-demo\nproject=old\nproject=/tmp/project=demo\nkind=scout\nmalformed\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	meta, err := ParseMeta(path)
	if err != nil {
		t.Fatalf("ParseMeta() error = %v", err)
	}
	if meta.ID != "demo" {
		t.Fatalf("ParseMeta() ID = %q, want demo", meta.ID)
	}
	if meta.Project != "/tmp/project=demo" {
		t.Fatalf("ParseMeta() Project = %q, want embedded equals preserved", meta.Project)
	}
	if meta.Kind != "scout" {
		t.Fatalf("ParseMeta() Kind = %q, want scout", meta.Kind)
	}
}

func TestLoadTaskMetadataSortsByTaskID(t *testing.T) {
	state := t.TempDir()
	for name, content := range map[string]string{
		"zeta.meta":  "kind=ship\n",
		"alpha.meta": "kind=scout\n",
	} {
		if err := os.WriteFile(filepath.Join(state, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	metas, err := LoadTaskMetadata(state)
	if err != nil {
		t.Fatalf("LoadTaskMetadata() error = %v", err)
	}
	if len(metas) != 2 || metas[0].ID != "alpha" || metas[1].ID != "zeta" {
		t.Fatalf("LoadTaskMetadata() IDs = %#v, want alpha,zeta", []string{metas[0].ID, metas[1].ID})
	}
}
