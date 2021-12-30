package tunchannel

import "testing"

func TestToCIDR(t *testing.T) {
	type args struct {
		s string
	}

	tests := []struct {
		name     string
		args     args
		wantCidr string
		wantErr  bool
	}{
		{
			"",
			args{
				s: "192.168.31.2",
			},
			"192.168.31.2/24",
			false,
		},
		{
			"",
			args{
				s: "192.168.31.2/24",
			},
			"192.168.31.2/24",
			false,
		},
		{
			"",
			args{
				s: "192.168.31.2/20",
			},
			"192.168.31.2/20",
			false,
		},
		{
			"",
			args{
				s: "192.168.31.2000",
			},
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCidr, err := ToCIDR(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToCIDR() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if gotCidr != tt.wantCidr {
				t.Errorf("ToCIDR() gotCidr = %v, want %v", gotCidr, tt.wantCidr)
			}
		})
	}
}
