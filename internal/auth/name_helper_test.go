package auth

import "testing"

func TestUserDisplayName(t *testing.T) {
	tests := []struct {
		first       string
		last        string
		email       string
		wantName    string
		wantInitial string
	}{
		{
			first:       "Angel",
			last:        "Velasco",
			email:       "foo@bar.com",
			wantName:    "Angel Velasco",
			wantInitial: "A",
		},
		{
			first:       "",
			last:        "",
			email:       "angel.emma.velasco@gmail.com",
			wantName:    "Angel Velasco",
			wantInitial: "A",
		},
		{
			first:       "",
			last:        "",
			email:       "angel.moreno5504@alumnos.udg.mx",
			wantName:    "Angel Moreno",
			wantInitial: "A",
		},
		{
			first:       "",
			last:        "",
			email:       "m.velasco@gmail.com",
			wantName:    "M Velasco",
			wantInitial: "M",
		},
		{
			first:       "",
			last:        "",
			email:       "usuario@example.com",
			wantName:    "Usuario",
			wantInitial: "U",
		},
		{
			first:       "",
			last:        "",
			email:       "development@thinker.local",
			wantName:    "Development",
			wantInitial: "D",
		},
		{
			first:       "",
			last:        "",
			email:       "",
			wantName:    "Usuario",
			wantInitial: "U",
		},
	}

	for _, tt := range tests {
		gotName, gotInitial := UserDisplayName(tt.first, tt.last, tt.email)
		if gotName != tt.wantName || gotInitial != tt.wantInitial {
			t.Errorf("UserDisplayName(%q, %q, %q) = (%q, %q), want (%q, %q)",
				tt.first, tt.last, tt.email, gotName, gotInitial, tt.wantName, tt.wantInitial)
		}
	}
}
