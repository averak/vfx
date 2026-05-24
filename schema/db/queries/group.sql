-- name: GetGroup :one
SELECT * FROM groups
WHERE id = $1;

-- name: UpsertGroup :exec
INSERT INTO groups (id, name, owner_id, founded_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name;

-- name: DeleteGroup :exec
DELETE FROM groups
WHERE id = $1;

-- name: AddGroupMember :exec
INSERT INTO group_members (id, group_id, player_id, joined_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (group_id, player_id) DO NOTHING;

-- name: RemoveGroupMember :execrows
DELETE FROM group_members
WHERE group_id = $1 AND player_id = $2;

-- name: GroupMemberExists :one
SELECT EXISTS (
  SELECT 1 FROM group_members
  WHERE group_id = $1 AND player_id = $2
);

-- name: ListGroupsForPlayer :many
SELECT g.*
FROM groups g
JOIN group_members m ON m.group_id = g.id
WHERE m.player_id = $1
ORDER BY g.founded_at DESC;

-- name: ListGroupMembers :many
SELECT p.id, p.nickname, m.joined_at
FROM group_members m
JOIN players p ON p.id = m.player_id
WHERE m.group_id = $1
ORDER BY m.joined_at ASC;
