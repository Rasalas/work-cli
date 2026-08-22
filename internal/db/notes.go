package db

import (
	"context"
	"time"
)

type Note struct {
	ID        int64
	SessionID int64
	Kind      string
	Body      string
	CreatedAt time.Time
}

type NoteUpdate struct {
	Kind      *string
	Body      *string
	CreatedAt *time.Time
}

func (s *Store) AddNote(ctx context.Context, kind, body string, createdAt time.Time) (Note, error) {
	running, err := s.RunningSession(ctx)
	if err != nil {
		return Note{}, err
	}
	if running == nil {
		return Note{}, ErrNoRunningSession
	}
	return s.insertNote(ctx, running.ID, kind, body, createdAt)
}

func (s *Store) AddNoteToSession(ctx context.Context, sessionID int64, kind, body string, createdAt time.Time) (Note, error) {
	if _, err := s.SessionByID(ctx, sessionID); err != nil {
		return Note{}, err
	}
	return s.insertNote(ctx, sessionID, kind, body, createdAt)
}

func (s *Store) insertNote(ctx context.Context, sessionID int64, kind, body string, createdAt time.Time) (Note, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO notes (session_id, kind, body, created_at)
VALUES (?, ?, ?, ?)
`, sessionID, kind, body, formatTime(createdAt))
	if err != nil {
		return Note{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Note{}, err
	}
	return Note{ID: id, SessionID: sessionID, Kind: kind, Body: body, CreatedAt: createdAt}, nil
}

func (s *Store) NotesForSession(ctx context.Context, sessionID int64) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, kind, body, created_at
FROM notes
WHERE session_id = ?
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var note Note
		if err := rows.Scan(&note.ID, &note.SessionID, &note.Kind, &note.Body, parseScanner(&note.CreatedAt)); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (s *Store) NoteByID(ctx context.Context, id int64) (Note, error) {
	var note Note
	err := s.db.QueryRowContext(ctx, `
SELECT id, session_id, kind, body, created_at
FROM notes
WHERE id = ?
`, id).Scan(&note.ID, &note.SessionID, &note.Kind, &note.Body, parseScanner(&note.CreatedAt))
	if err != nil {
		return Note{}, err
	}
	return note, nil
}

func (s *Store) UpdateNote(ctx context.Context, id int64, update NoteUpdate) (Note, error) {
	note, err := s.NoteByID(ctx, id)
	if err != nil {
		return Note{}, err
	}

	kind := note.Kind
	if update.Kind != nil {
		kind = *update.Kind
	}
	body := note.Body
	if update.Body != nil {
		body = *update.Body
	}
	createdAt := note.CreatedAt
	if update.CreatedAt != nil {
		createdAt = *update.CreatedAt
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE notes
SET kind = ?, body = ?, created_at = ?
WHERE id = ?
`, kind, body, formatTime(createdAt), id)
	if err != nil {
		return Note{}, err
	}
	return s.NoteByID(ctx, id)
}
