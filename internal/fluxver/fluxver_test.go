package fluxver

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in        string
		want      string
		supported bool
		wantErr   bool
	}{
		{in: "flux version 2.9.1", want: "2.9.1", supported: true},
		{in: "flux version 2.7.0", want: "2.7.0", supported: true},
		{in: "flux version 2.8.3\n", want: "2.8.3", supported: true},
		{in: "flux version 2.3.0", want: "2.3.0", supported: false},
		{in: "flux version 3.0.0", want: "3.0.0", supported: false},
		{in: "no version here", wantErr: true},
	}
	for _, c := range cases {
		v, err := Parse(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Parse(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if v.String() != c.want {
			t.Errorf("Parse(%q) = %s, want %s", c.in, v, c.want)
		}
		if v.Supported() != c.supported {
			t.Errorf("Parse(%q).Supported() = %v, want %v", c.in, v.Supported(), c.supported)
		}
	}
}

func TestAPIs(t *testing.T) {
	apis := Version{Major: 2, Minor: 9}.APIs()
	if apis.HelmRelease != "helm.toolkit.fluxcd.io/v2" {
		t.Errorf("HelmRelease = %s", apis.HelmRelease)
	}
	if apis.Kustomize != "kustomize.toolkit.fluxcd.io/v1" {
		t.Errorf("Kustomize = %s", apis.Kustomize)
	}
}
