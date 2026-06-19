package messages

import (
	"reflect"
	"testing"
)

func TestEventSubjectsPreserveRepositoryNesting(t *testing.T) {
	got := EventSubjects("gelato", EventRepositoryAdded, "platform/api")
	want := []string{
		"gelato.events.repo.platform.api.repository.added",
		"gelato.events.type.repository.added.platform.api",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EventSubjects() = %#v, want %#v", got, want)
	}
}

func TestRepoSubjectTokensEncodeUnsafeCharactersPerPathSegment(t *testing.T) {
	got := RepoSubjectTokens("/platform/api.v2/my repo/")
	want := []string{"platform", "api%2Ev2", "my%20repo"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RepoSubjectTokens() = %#v, want %#v", got, want)
	}
}
