-- +goose Up
CREATE TABLE chirps(id uuid primary key, created_at timestamp not null, updated_at timestamp not null, body text not null, user_id uuid,
constraint fk_user
	foreign key (user_id) references users(id)
	on delete cascade);

-- +goose Down
DROP TABLE chirps;
