-- +goose Up
-- Avalon game backend initial schema

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Rooms represent persistent lobbies that can host many games.
CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rooms_code ON rooms (code);

-- Players within a room. Anonymous identities scoped per room.
CREATE TABLE room_players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    is_host BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_room_players_unique_name
    ON room_players (room_id, display_name);

-- Individual games that occur within a room.
CREATE TABLE games (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    status TEXT NOT NULL, -- waiting | in_progress | finished
    config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ
);

CREATE INDEX idx_games_room_created_at ON games (room_id, created_at DESC);

-- A room player participating in a specific game.
CREATE TABLE game_players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    game_id UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    room_player_id UUID NOT NULL REFERENCES room_players(id) ON DELETE CASCADE,
    role TEXT,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_game_players_unique_room_player
    ON game_players (game_id, room_player_id);

-- Snapshots of full game state for fast reload and reconnection.
CREATE TABLE game_state_snapshots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    game_id UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    state_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_game_state_snapshots_version
    ON game_state_snapshots (game_id, version);

-- Append-only event/move log.
CREATE TABLE game_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    game_id UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    room_player_id UUID REFERENCES room_players(id) ON DELETE SET NULL,
    type TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_game_events_game_created_at
    ON game_events (game_id, created_at);

-- Optional separate chat messages table.
CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    game_id UUID REFERENCES games(id) ON DELETE CASCADE,
    room_player_id UUID NOT NULL REFERENCES room_players(id) ON DELETE CASCADE,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_messages_room_created_at
    ON chat_messages (room_id, created_at);

-- +goose Down
-- Drop all tables in reverse order of creation
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS game_events;
DROP TABLE IF EXISTS game_state_snapshots;
DROP TABLE IF EXISTS game_players;
DROP TABLE IF EXISTS games;
DROP TABLE IF EXISTS room_players;
DROP TABLE IF EXISTS rooms;
DROP EXTENSION IF EXISTS "uuid-ossp";
