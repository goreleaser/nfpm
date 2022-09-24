package rpmpack

import (
	"testing"
)

func TestFileTypeSetting(t *testing.T) {
	f := &RPMFile{
		Name: "Test",
	}

	if f.Type != GenericFile {
		t.Error("New RPMFile.Type should be a generic type")
	}

	f.Type |= ConfigFile
	if (f.Type & ConfigFile) == 0 {
		t.Error("Setting to config file should have the ConfigFile bitmask")
	}
}

func TestFileTypeCombining(t *testing.T) {
	f := &RPMFile{
		Name: "Test",
	}

	f.Type |= ConfigFile | NoReplaceFile

	if (f.Type&ConfigFile) == 0 || f.Type&NoReplaceFile == 0 {
		t.Error("Combining file types should have the bitmask of both")
	}
}
