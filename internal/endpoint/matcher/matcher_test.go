package matcher

import "testing"

func TestMatchers(t *testing.T) {
	tests := []struct {
		name    string
		matcher Matcher
		value   string
		want    bool
	}{
		{name: "exact hit", matcher: NewExact([]string{"443", "8443"}), value: "443", want: true},
		{name: "exact miss", matcher: NewExact([]string{"443", "8443"}), value: "80", want: false},
		{name: "prefix hit", matcher: NewPrefix([]string{"/tmp/", "/dev/shm/"}), value: "/tmp/payload", want: true},
		{name: "prefix miss", matcher: NewPrefix([]string{"/tmp/", "/dev/shm/"}), value: "/var/log/app", want: false},
		{name: "suffix hit", matcher: NewSuffix([]string{".sh", ".so"}), value: "/tmp/x.sh", want: true},
		{name: "suffix miss", matcher: NewSuffix([]string{".sh", ".so"}), value: "/tmp/x.py", want: false},
		{name: "contains hit", matcher: NewContains([]string{" curl ", " wget "}), value: "/bin/sh -c curl http://example.invalid", want: true},
		{name: "contains miss", matcher: NewContains([]string{" curl ", " wget "}), value: "/usr/bin/python app.py", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.matcher.Match(tt.value); got != tt.want {
				t.Fatalf("Match() = %t, want %t", got, tt.want)
			}
		})
	}
}
