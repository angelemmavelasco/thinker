package models

import "thinker/internal/database"

type Idea struct {
	User database.User
	Idea database.Idea
}
