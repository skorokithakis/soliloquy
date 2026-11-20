package main

import (
	"context"
	"fmt"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

// voteCounts and threadVoteCounts vary engagement so the seeded pages show
// different scores instead of a uniform wall of ones.
var (
	voteCounts       = []int{3, 1, 0, 5, 2, 7, 4, 6, 1, 2, 8, 3}
	threadVoteCounts = []int{4, 2, 6, 1, 3, 5, 0, 7, 2, 4, 1, 3}
	reactionCycle    = []db.Reaction{
		"", db.ReactionHeart, "", db.ReactionIdea, "",
		db.ReactionLaugh, db.ReactionThanks, "", db.ReactionParty, "",
	}
)

type seededComment struct {
	id     int64
	author string
}

type seeder struct {
	db     *db.DB
	now    time.Time
	users  map[string]*db.User
	voters []*db.User
	hashes map[string]string
}

// seed writes the QA forum into an empty database. It returns false without
// changing anything when a thread already exists, which makes repeated runs of
// .symphony/serve cheap and keeps their dates stable.
func seed(ctx context.Context, database *db.DB, now time.Time) (bool, error) {
	// Count soft-deleted threads too: once anyone has created a thread, even
	// one they later deleted, the database is no longer the empty forum this
	// seeder is meant to fill.
	var threadsExist bool
	if err := database.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM threads)`,
	).Scan(&threadsExist); err != nil {
		return false, fmt.Errorf("check for threads: %w", err)
	}
	if threadsExist {
		return false, nil
	}

	s := &seeder{
		db:     database,
		now:    now,
		users:  make(map[string]*db.User, len(qaUsers)),
		hashes: make(map[string]string, len(qaUsers)),
	}
	if err := s.createUsers(ctx); err != nil {
		return false, err
	}
	if err := s.createThreads(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *seeder) createUsers(ctx context.Context) error {
	for _, spec := range qaUsers {
		username := auth.NormalizeUsername(spec.username)

		existing, err := s.db.UserByUsername(ctx, username)
		if err != nil {
			return fmt.Errorf("look up user %q: %w", username, err)
		}
		if existing != nil {
			s.users[username] = existing
			s.voters = append(s.voters, existing)
			continue
		}

		hash, ok := s.hashes[spec.password]
		if !ok {
			hash, err = auth.HashPassword(spec.password)
			if err != nil {
				return fmt.Errorf("hash password for %q: %w", username, err)
			}
			s.hashes[spec.password] = hash
		}

		var invitedBy *int64
		if spec.invitedBy != "" {
			parent, ok := s.users[spec.invitedBy]
			if !ok {
				return fmt.Errorf("inviter %q for %q does not exist", spec.invitedBy, username)
			}
			invitedBy = &parent.ID
		}

		user, err := s.db.CreateUser(ctx, username, hash, spec.isAdmin, invitedBy, s.now.Add(-spec.age))
		if err != nil {
			return fmt.Errorf("create user %q: %w", username, err)
		}
		s.users[username] = user
		s.voters = append(s.voters, user)
	}
	return nil
}

func (s *seeder) createThreads(ctx context.Context) error {
	for i, spec := range qaThreads() {
		author, ok := s.users[spec.author]
		if !ok {
			return fmt.Errorf("thread %q has unknown author %q", spec.title, spec.author)
		}

		bodyHTML := ""
		if spec.body != "" {
			rendered, err := markdown.RenderThread(spec.body)
			if err != nil {
				return fmt.Errorf("render thread %q: %w", spec.title, err)
			}
			bodyHTML = string(rendered)
		}

		thread, err := s.db.CreateThread(ctx, author.ID, spec.title, spec.url, spec.body, bodyHTML, s.now.Add(-spec.age))
		if err != nil {
			return fmt.Errorf("create thread %q: %w", spec.title, err)
		}

		comments, err := s.createComments(ctx, thread, spec.comments)
		if err != nil {
			return fmt.Errorf("create comments for %q: %w", spec.title, err)
		}

		if err := s.applyEngagement(ctx, thread, comments, i); err != nil {
			return fmt.Errorf("add votes for %q: %w", spec.title, err)
		}

		for j, comment := range spec.comments {
			if !comment.deleted {
				continue
			}
			deleted, err := s.db.DeleteComment(ctx, comments[j].id, s.now.Add(-time.Hour))
			if err != nil {
				return fmt.Errorf("delete comment on %q: %w", spec.title, err)
			}
			if !deleted {
				return fmt.Errorf("delete comment on %q: comment %d was already deleted", spec.title, comments[j].id)
			}
		}
	}
	return nil
}

func (s *seeder) createComments(ctx context.Context, thread *db.Thread, specs []commentSpec) ([]seededComment, error) {
	comments := make([]seededComment, 0, len(specs))

	for i, spec := range specs {
		author, ok := s.users[spec.author]
		if !ok {
			return nil, fmt.Errorf("comment %d has unknown author %q", i, spec.author)
		}

		var parentID *int64
		if spec.parent >= 0 {
			if spec.parent >= i {
				return nil, fmt.Errorf("comment %d has parent %d, which comes later", i, spec.parent)
			}
			parent := comments[spec.parent].id
			parentID = &parent
		}

		bodyHTML, err := markdown.Render(spec.body)
		if err != nil {
			return nil, fmt.Errorf("render comment %d: %w", i, err)
		}

		createdAt := s.now.Add(-spec.age)
		comment, err := s.db.CreateComment(ctx, thread.ID, parentID, author.ID, spec.body, string(bodyHTML), createdAt)
		if err != nil {
			return nil, fmt.Errorf("create comment %d: %w", i, err)
		}

		if spec.edit != "" {
			editedHTML, err := markdown.Render(spec.edit)
			if err != nil {
				return nil, fmt.Errorf("render edited comment %d: %w", i, err)
			}
			editedAt := createdAt.Add(2 * time.Hour)
			if !editedAt.Before(s.now) {
				editedAt = s.now.Add(-time.Minute)
			}
			updated, err := s.db.UpdateComment(ctx, comment.ID, author.ID, spec.edit, string(editedHTML), editedAt)
			if err != nil {
				return nil, fmt.Errorf("edit comment %d: %w", i, err)
			}
			if !updated {
				return nil, fmt.Errorf("edit comment %d: no row updated", i)
			}
		}

		comments = append(comments, seededComment{id: comment.ID, author: author.Username})
	}

	return comments, nil
}

// applyEngagement adds thread votes, comment votes and reactions. Voters are
// chosen in a deterministic rotation so repeated seeds (on a fresh database)
// produce the same scores. The author of an item is skipped because they
// already have their own vote row from creation, and voting would remove it.
func (s *seeder) applyEngagement(ctx context.Context, thread *db.Thread, comments []seededComment, threadIndex int) error {
	threadVoters := s.pickVoters(thread.AuthorUsername, threadIndex, threadVoteCounts[threadIndex%len(threadVoteCounts)])
	for _, voter := range threadVoters {
		if _, err := s.db.ToggleThreadVote(ctx, voter.ID, thread.ID); err != nil {
			return fmt.Errorf("thread %d vote by %q: %w", thread.ID, voter.Username, err)
		}
	}

	for i, comment := range comments {
		count := voteCounts[(i+threadIndex)%len(voteCounts)]
		voters := s.pickVoters(comment.author, i+threadIndex, count)
		for j, voter := range voters {
			if j == 0 {
				if reaction := reactionCycle[(i+threadIndex)%len(reactionCycle)]; reaction != "" {
					if _, err := s.db.SetCommentReaction(ctx, voter.ID, comment.id, reaction); err != nil {
						return fmt.Errorf("comment %d reaction by %q: %w", comment.id, voter.Username, err)
					}
					continue
				}
			}
			if j == 1 && count >= 4 {
				reaction := db.CommentReactions[(i+2)%len(db.CommentReactions)]
				if _, err := s.db.SetCommentReaction(ctx, voter.ID, comment.id, reaction); err != nil {
					return fmt.Errorf("comment %d second reaction by %q: %w", comment.id, voter.Username, err)
				}
				continue
			}
			if _, err := s.db.ToggleCommentVote(ctx, voter.ID, comment.id); err != nil {
				return fmt.Errorf("comment %d vote by %q: %w", comment.id, voter.Username, err)
			}
		}
	}

	return nil
}

// pickVoters returns up to count accounts, starting at start in the account
// list and skipping exclude. There are nine accounts, so eight voters fit even
// when the author is excluded.
func (s *seeder) pickVoters(exclude string, start, count int) []*db.User {
	if count > len(s.voters)-1 {
		count = len(s.voters) - 1
	}

	voters := make([]*db.User, 0, count)
	for offset := 0; offset < len(s.voters) && len(voters) < count; offset++ {
		user := s.voters[(start+offset)%len(s.voters)]
		if user.Username == exclude {
			continue
		}
		voters = append(voters, user)
	}
	return voters
}
