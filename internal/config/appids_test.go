package config

import (
	"reflect"
	"testing"
)

func TestDesktopAppIDConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        []string
		invalid     bool
	}{
		{"scalar", "org.example.One", []string{"org.example.One"}, false},
		{"ordered list", "[org.example.One, org.example.Two]", []string{"org.example.One", "org.example.Two"}, false},
		{"empty list", "[]", []string{}, false},
		{"mapping", "{bad: value}", nil, true},
		{"number in list", "[org.example.One, 3]", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfigFile(t, "system_page:\n  health_group:\n    app_id: "+tc.value+"\n")
			cfg, err := loadFromPath(path)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid app_id accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.SystemPage["health_group"].AppIDs; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("IDs = %#v, want %#v", got, tc.want)
			}
		})
	}
}
