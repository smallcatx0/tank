package dao

import "testing"

func TestDsnMask(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "normal",
			dsn:  "root:pass@tcp(127.0.0.1:3306)/bs",
			want: "root:******@tcp(127.0.0.1:3306)/bs",
		},
		{
			name: "empty",
			dsn:  "",
			want: "",
		},
		{
			name: "no_at_separator",
			dsn:  "invalid-dsn-without-at",
			want: "******",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("DsnMask(%q) panicked: %v", tt.dsn, r)
				}
			}()
			if got := DsnMask(tt.dsn); got != tt.want {
				t.Errorf("DsnMask(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}
