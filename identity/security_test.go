package identity

import (
	"strings"
	"testing"
)

// P1 Security: No dots in shade names — would break FQDN parsing.

func TestPalette_NoDots_InShadeNames(t *testing.T) {
	for _, family := range DefaultPalette() {
		if strings.Contains(family.Name, ".") {
			t.Errorf("family name %q contains dot — breaks FQDN", family.Name)
		}
		for _, shade := range family.Colors {
			if strings.Contains(shade.Name, ".") {
				t.Errorf("shade name %q in family %s contains dot — breaks FQDN", shade.Name, family.Name)
			}
		}
	}
}

// P1 Security: No spaces in shade names — FQDN should be URL-safe.

func TestPalette_NoSpaces_InShadeNames(t *testing.T) {
	for _, family := range DefaultPalette() {
		if strings.Contains(family.Name, " ") {
			t.Errorf("family name %q contains space", family.Name)
		}
		for _, shade := range family.Colors {
			if strings.Contains(shade.Name, " ") {
				t.Errorf("shade name %q in family %s contains space", shade.Name, family.Name)
			}
		}
	}
}

// P1: All shade names are lowercase.

func TestPalette_AllLowercase(t *testing.T) {
	for _, family := range DefaultPalette() {
		if family.Name != strings.ToLower(family.Name) {
			t.Errorf("family %q not lowercase", family.Name)
		}
		for _, shade := range family.Colors {
			if shade.Name != strings.ToLower(shade.Name) {
				t.Errorf("shade %q in %s not lowercase", shade.Name, family.Name)
			}
		}
	}
}
