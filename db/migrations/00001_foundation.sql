-- +goose Up
CREATE EXTENSION vector;
CREATE SCHEMA core;
CREATE SCHEMA retrieval;
CREATE SCHEMA telemetry;

-- +goose Down
DROP SCHEMA telemetry;
DROP SCHEMA retrieval;
DROP SCHEMA core;
DROP EXTENSION vector;
