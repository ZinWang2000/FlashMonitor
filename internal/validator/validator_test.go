package validator

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"valid_name", false},
		{"valid-name.1", false},
		{"UPPERCASE", false},
		{"mixed123", false},
		{"", true},
		{"has space", true},
		{"has/slash", true},
		{"has;semi", true},
		{strings.Repeat("a", 129), true},
	}
	for _, c := range cases {
		err := ValidateName(c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateName(%q) error=%v, wantErr=%v", c.name, err, c.wantErr)
		}
	}
}

func TestValidateOutputPath(t *testing.T) {
	baseDir := t.TempDir()

	got, err := ValidateOutputPath("", baseDir)
	if err != nil || got != baseDir {
		t.Errorf("empty input: got %q, %v", got, err)
	}

	got, err = ValidateOutputPath(baseDir+"/subdir/file.csv", baseDir)
	if err != nil {
		t.Errorf("valid subpath: %v", err)
	}
	_ = got

	_, err = ValidateOutputPath(baseDir+"/../secret", baseDir)
	if err == nil {
		t.Error("path traversal: expected error, got nil")
	}
}
