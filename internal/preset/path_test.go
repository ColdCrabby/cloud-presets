package preset

import "testing"

func TestKindFromPath(t *testing.T) {
	cases := []struct {
		path string
		want Kind
		ok   bool
	}{
		{"vendors/prusa/printers/prusa-mk4-0.4.yaml", KindPrinter, true},
		{"vendors/prusa/filaments/prusament-pla.yaml", KindFilament, true},
		{"processes/coldcrabby-standard-0.20.yaml", KindProcess, true},
		{"./vendors/bambu/printers/x1c.yaml", KindPrinter, true},
		{`vendors\bambu\filaments\pla.yaml`, KindFilament, true},
		{"vendors/prusa/vendor.yaml", "", false},
		{"random/place/file.yaml", "", false},
	}
	for _, tc := range cases {
		got, ok := KindFromPath(tc.path)
		if got != tc.want || ok != tc.ok {
			t.Errorf("KindFromPath(%q) = (%q, %v), want (%q, %v)", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}
