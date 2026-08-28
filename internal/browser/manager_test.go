package browser

import "testing"

func TestIsSensitiveActionLogout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
		name string
		want bool
	}{
		{role: "menuitem", name: "Sair de @venn_mak", want: true},
		{role: "button", name: "Log out @venn_mak", want: true},
		{role: "link", name: "Sign out", want: true},
		{role: "button", name: "Logout", want: true},
		{role: "button", name: "Abrir menu da conta", want: false},
		{role: "textbox", name: "Sair", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.role+"/"+test.name, func(t *testing.T) {
			t.Parallel()
			if got := isSensitiveAction(test.role, test.name); got != test.want {
				t.Fatalf("isSensitiveAction(%q, %q) = %v; want %v", test.role, test.name, got, test.want)
			}
		})
	}
}
