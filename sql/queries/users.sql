-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, name)
VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING *;

-- name: GetUser :one
SELECT *
FROM users
WHERE name = $1;

-- name: GetFeed :one
SELECT *
FROM feeds
WHERE url = $1;

-- name: GetFeedFollowsForUser :many
SELECT feed_follows.*, feeds.name AS feed_name, users.name AS user_name
FROM feed_follows
INNER JOIN feeds ON feed_follows.feed_id = feeds.id
INNER JOIN users ON feed_follows.user_id = users.id;

-- name: Reset :exec
DELETE FROM users
WHERE name is Not NULL;

-- name: GetUsers :many
SELECT name
FROM users;

-- name: CreateFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
)
RETURNING *;

-- name: GetFeeds :many
SELECT feeds.name, feeds.url, users.name AS created_by
FROM feeds 
LEFT JOIN users ON feeds.user_id = users.id;

-- name: CreateFeedFollow :one
WITH inserted_feed_follow AS (
    INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING *
) SELECT inserted_feed_follow.*, feeds.name AS feed_name, users.name AS user_name
FROM inserted_feed_follow
INNER JOIN feeds ON inserted_feed_follow.feed_id = feeds.id
INNER JOIN users ON inserted_feed_follow.user_id = users.id;


-- name: Unfollow :one
DELETE FROM feed_follows
WHERE user_id IN (
    SELECT id
    FROM users
    WHERE users.name = $1
)
AND feed_id IN (
    SELECT id
    FROM feeds
    WHERE feeds.url = $2
)
RETURNING *;

-- name: MarkFeedFetched :one
UPDATE feeds
SET last_fetched_at = $1, updated_at = $1
WHERE id = $2
RETURNING *;

-- name: GetNextFeedToFetch :one
SELECT *
FROM feeds
ORDER BY last_fetched_at NULLS FIRST
LIMIT 1;


-- name: CreatePost :one
WITH inserted_posts AS (
    INSERT INTO posts (id, created_at, updated_at, title, url, description, published_at, feed_id)
    VALUES (
        $1,
        $2,
        $3,
        $4,
        $5,
        $6,
        $7,
        $8
    )
    RETURNING *
) SELECT inserted_posts.*
FROM inserted_posts
INNER JOIN feeds ON inserted_posts.feed_id = feeds.id;

-- name: GetPostsForUsers :many
SELECT posts.*
FROM posts
INNER JOIN feed_follows ON feed_follows.feed_id = posts.feed_id
WHERE feed_follows.user_id = $1
ORDER BY posts.published_at DESC
LIMIT $2;