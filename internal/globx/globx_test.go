package globx

import "testing"

func TestMatchDoubleStarExtension(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.ts", "main.ts", true},
		{"**/*.ts", "src/a/b/main.ts", true},
		{"**/*.ts", "src/a/b/main.go", false},
		{"*.tsx", "root.tsx", true},
		{"*.tsx", "src/root.tsx", false},
		{"frontend/src/**/*.tsx", "frontend/src/app/root.tsx", true},
		{"frontend/src/**/*.tsx", "frontend/root.tsx", false},
		{"**/nested/*.tsx", "a/nested/root.tsx", true},
		{"**/nested/*.tsx", "nested/root.tsx", true},
		{"**/nested/*.tsx", "a/root.tsx", false},
		{"**/prisma/**", "prisma/schema.prisma", true},
		{"**/prisma/**", "backend/prisma/migrations/1.sql", true},
		{"**/prisma/**", "backend/src/main.ts", false},
		{"**/*.java", "src/main/java/App.java", true},
		{"src/main/java/**/*.java", "src/main/java/com/x/App.java", true},
		{"src/main/java/**/*.java", "src/test/java/App.java", false},
		{"**/*Test.java", "src/FooTest.java", true},
		{"**/__tests__/**", "pkg/__tests__/a.test.ts", true},
		{"**/*.test.ts", "pkg/a.test.ts", true},
		{"**/*.test.ts", "pkg/a.test.tsx", false},
		{"**/*.component.ts", "frontend/src/app/x.component.ts", true},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.path); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"**/*.ts", "**/*.tsx"}
	if !MatchAny(patterns, "src/a.ts") {
		t.Fatal("MatchAny = false for **/*.ts match")
	}
	if !MatchAny(patterns, "src/a.tsx") {
		t.Fatal("MatchAny = false for **/*.tsx match")
	}
	if MatchAny(patterns, "src/a.go") {
		t.Fatal("MatchAny = true for non-match")
	}
	if MatchAny(nil, "src/a.ts") {
		t.Fatal("MatchAny = true with no patterns")
	}
}
