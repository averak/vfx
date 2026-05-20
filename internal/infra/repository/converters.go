// Package repository contains the storage implementations of the
// domain Repository interfaces.
//
// Converters here translate between sqlc's pgtype values and Go-native
// types (time.Time, pointer-to-time). Keeping the translation in one
// file lets the rest of the package read like straight domain code.
package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func fromNullableTimestamptz(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
