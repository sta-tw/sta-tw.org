//go:build integration

package content

import (
	"context"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestListSpacesSupportsGlobalSpaceWithNullableScope(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repository, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	spaces, err := repository.ListSpaces(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, space := range spaces {
		if space.SpaceType != "global" {
			continue
		}
		if space.AcademicYear != nil || space.SchoolCode != "" || space.ProgramCode != "" {
			t.Fatalf("global forum space has unexpected scope: %#v", space)
		}
		return
	}
	t.Fatal("global forum space was not returned")
}
