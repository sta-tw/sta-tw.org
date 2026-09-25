package schools

import "testing"

func TestSearchSupportsCanonicalAndAbbreviatedQueries(t *testing.T) {
	items := []School{
		{SchoolCode: "003", SchoolName: "國立臺灣大學", InstitutionType: GeneralUniversity, IsActive: true},
		{SchoolCode: "032", SchoolName: "臺北市立大學", InstitutionType: GeneralUniversity, IsActive: true},
		{SchoolCode: "036", SchoolName: "國立臺北科技大學", InstitutionType: TechnicalInstitution, IsActive: true},
		{SchoolCode: "150", SchoolName: "測試停用學校", InstitutionType: GeneralUniversity, IsActive: false},
	}

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "canonical name", query: "國立臺灣大學", want: "003"},
		{name: "traditional variant", query: "台大", want: "003"},
		{name: "ordered abbreviation", query: "北科", want: "036"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results := Search(items, test.query, 10)
			if len(results) == 0 || results[0].SchoolCode != test.want {
				t.Fatalf("Search(%q) = %#v, want %s first", test.query, results, test.want)
			}
		})
	}

	if results := Search(items, "停用", 10); len(results) != 0 {
		t.Fatalf("public search returned inactive schools: %#v", results)
	}
	if results := SearchAll(items, "停用", 10); len(results) != 1 || results[0].SchoolCode != "150" {
		t.Fatalf("admin search = %#v, want inactive school", results)
	}
}

func TestSearchReturnsManyCandidatesWhileTyping(t *testing.T) {
	items := []School{
		{SchoolCode: "001", SchoolName: "國立臺灣大學", IsActive: true},
		{SchoolCode: "002", SchoolName: "國立臺灣師範大學", IsActive: true},
		{SchoolCode: "032", SchoolName: "臺北市立大學", IsActive: true},
	}
	results := Search(items, "大", 30)
	if len(results) != 3 {
		t.Fatalf("Search should retain all candidates for a broad query, got %d", len(results))
	}
}
