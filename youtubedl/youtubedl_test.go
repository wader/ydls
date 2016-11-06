package youtubedl

import (
	"encoding/json"
	"fmt"
	"go/build"
	"path/filepath"
	"reflect"
	"testing"
)

// this will only work if tests are run where they are built
func mustPkgSourcePath() string {
	type dummy struct{}
	pkgPath := reflect.TypeOf(dummy{}).PkgPath()
	srcDirs := build.Default.SrcDirs()
	for i := len(srcDirs) - 1; i >= 0; i-- {
		pkg, _ := build.Import(pkgPath, srcDirs[i], build.FindOnly)
		if pkg != nil {
			return pkg.Dir
		}
	}

	panic(fmt.Sprintf("could not find source path for %s", pkgPath))
}

func TestParseInfo(t *testing.T) {
	pkgSourcePath := mustPkgSourcePath()
	testPath := filepath.Join(pkgSourcePath, "test")

	for _, c := range []struct {
		name          string
		expectedTitle string
	}{
		{"soundcloud.com", "BIS Radio Show #793 with The Drifter"},
		{"vimeo.com", "Ben Nagy Fuzzing OSX At Scale"},
		{"www.infoq.com", "Simple Made Easy"},
		{"www.svtplay.se", "Avsnitt 1"},
		{"www.youtube.com", "A Radiolab Producer on the Making of a Podcast"},
	} {
		yi, err := NewFromPath(filepath.Join(testPath, c.name))
		if err != nil {
			t.Errorf("failed to parse %s", c.name)
		}

		if yi.Title != c.expectedTitle {
			t.Errorf("%s expected title '%s' got '%s'", c.name, c.expectedTitle, yi.Title)
		}

		if yi.Thumbnail != "" && len(yi.ThumbnailBytes) == 0 {
			t.Errorf("%s expected thumbnail bytes", c.name)
		}

		var dummy map[string]interface{}
		if err := json.Unmarshal(yi.rawJSON, &dummy); err != nil {
			t.Errorf("%s failed to parse rawJSON", c.name)
		}

		if len(yi.Formats) == 0 {
			t.Errorf("%s expected formats", c.name)
		}

		for _, f := range yi.Formats {
			if f.FormatID == "" {
				t.Errorf("%s %s expected FormatID not empty", c.name, f.FormatID)
			}
			if f.ACodec != "" && f.ACodec != "none" && f.Ext != "" && f.NormACodec == "" {
				t.Errorf("%s %s expected NormACodec not empty for %s", c.name, f.FormatID, f.ACodec)
			}
			if f.VCodec != "" && f.VCodec != "none" && f.Ext != "" && f.NormVCodec == "" {
				t.Errorf("%s %s expected NormVCodec not empty for %s", c.name, f.FormatID, f.VCodec)
			}
			if f.ABR+f.VBR+f.TBR != 0 && f.NormBR == 0 {
				t.Errorf("%s %s expected NormBR not zero", c.name, f.FormatID)
			}
		}
	}
}
