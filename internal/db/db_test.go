package db

import (
	"context"
	"testing"
)

func TestSavePageWithRevisionsHonorsLimit(t *testing.T) {
	store, err := Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()

	page := Page{Slug: "about", Path: "/about", Title: "Version 1", ContentType: "page", Markdown: "one", Published: true}
	if _, err := store.SavePageWithRevisions(ctx, page, 2); err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"Version 2", "Version 3", "Version 4"} {
		page.Title = title
		page.Markdown = title
		if _, err := store.SavePageWithRevisions(ctx, page, 2); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	revisions, err := store.ListPageRevisions(ctx, "about")
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revisions))
	}
	if revisions[0].Title != "Version 3" || revisions[1].Title != "Version 2" {
		t.Fatalf("unexpected revisions: %#v", revisions)
	}
}

func TestListPublishedPostsOrdersByPublishedAt(t *testing.T) {
	store, err := Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()

	pages := []Page{
		{Slug: "old-post", Path: "/blog/old-post", Title: "Old", ContentType: "post", Published: true, PublishedAt: "2026-01-02"},
		{Slug: "new-post", Path: "/blog/new-post", Title: "New", ContentType: "post", Published: true, PublishedAt: "2026-03-04"},
		{Slug: "draft-post", Path: "/blog/draft-post", Title: "Draft", ContentType: "post", Published: false, PublishedAt: "2026-04-05"},
		{Slug: "regular-page", Path: "/regular-page", Title: "Page", ContentType: "page", Published: true, PublishedAt: "2026-05-06"},
	}
	for _, page := range pages {
		if _, err := store.SavePage(ctx, page); err != nil {
			t.Fatalf("save %s: %v", page.Slug, err)
		}
	}

	posts, err := store.ListPublishedPosts(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	if posts[0].Slug != "new-post" || posts[1].Slug != "old-post" {
		t.Fatalf("unexpected post order: %#v", posts)
	}
}

func TestGetSettingsCacheReturnsCopies(t *testing.T) {
	store, err := Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()

	settings, err := store.GetSettings(ctx, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	settings.Menu[0].Label = "Mutated"

	again, err := store.GetSettings(ctx, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if again.Menu[0].Label != "Home" {
		t.Fatalf("cached settings menu was mutated: %#v", again.Menu)
	}
}

func TestDefaultSettingsUseDarkTheme(t *testing.T) {
	settings := DefaultSettings("Demo")
	if settings.AdminTheme != "dark" {
		t.Fatalf("expected dark admin theme, got %q", settings.AdminTheme)
	}
	if settings.DefaultTheme != "dark" {
		t.Fatalf("expected dark public theme, got %q", settings.DefaultTheme)
	}
}

func TestGetACLCacheReturnsCopiesAndInvalidatesOnSave(t *testing.T) {
	store, err := Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()

	_, rules, err := store.SaveACL(ctx, SecuritySettings{AdminDefault: "allow", PublicDefault: "allow"}, []ACLRule{
		{Scope: "admin", Action: "deny", CIDR: "203.0.113.0/24", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %#v", rules)
	}
	rules[0].CIDR = "198.51.100.0/24"

	_, again, err := store.GetACL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].CIDR != "203.0.113.0/24" {
		t.Fatalf("cached ACL rule was mutated: %#v", again)
	}

	_, updated, err := store.SaveACL(ctx, SecuritySettings{AdminDefault: "allow", PublicDefault: "allow"}, []ACLRule{
		{Scope: "admin", Action: "deny", CIDR: "198.51.100.0/24", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].CIDR != "198.51.100.0/24" {
		t.Fatalf("expected cache to refresh after save, got %#v", updated)
	}
}
