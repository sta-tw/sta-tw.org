package schools

import "testing"

func TestBatchInputValidation(t *testing.T) {
	active := true
	valid := BatchInput{
		Reason: "教育部名錄更新",
		Items: []SchoolInput{{
			SchoolCode: "003", SchoolName: "國立臺灣大學", InstitutionType: GeneralUniversity, IsActive: &active,
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}

	invalidCode := valid
	invalidCode.Items = []SchoolInput{{
		SchoolCode: "000", SchoolName: "國立臺灣大學", InstitutionType: GeneralUniversity, IsActive: &active,
	}}
	if err := invalidCode.Validate(); err == nil {
		t.Fatal("000 should be rejected by API validation")
	}

	missingActive := valid
	missingActive.Items = []SchoolInput{{
		SchoolCode: "003", SchoolName: "國立臺灣大學", InstitutionType: GeneralUniversity,
	}}
	if err := missingActive.Validate(); err == nil {
		t.Fatal("missing is_active should be rejected")
	}

	duplicate := valid
	duplicate.Items = []SchoolInput{
		{SchoolCode: "003", SchoolName: "國立臺灣大學", InstitutionType: GeneralUniversity, IsActive: &active},
		{SchoolCode: "003", SchoolName: "國立臺灣大學", InstitutionType: GeneralUniversity, IsActive: &active},
	}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate school codes should be rejected")
	}
}
