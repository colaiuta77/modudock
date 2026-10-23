// 인증과 코어 API의 실제 요청 및 응답 계약을 명세로 제공한다.
package core

import (
	"strings"
)

type apiOperation struct{ path, method, body, response, status, role, scope string }

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}
func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func arraySchema(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
func (s *Server) apiDocument() map[string]any {
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	date := map[string]any{"type": "string", "format": "date-time"}
	role := map[string]any{"type": "string", "enum": []string{"admin", "operator", "viewer"}}
	password := map[string]any{"type": "string", "minLength": 12, "maxLength": 1024, "writeOnly": true}
	username := map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_.-]{1,64}$"}
	user := objectSchema(map[string]any{"id": text, "username": username, "role": role, "is_active": boolean, "created_at": date}, "id", "username", "role", "is_active", "created_at")
	scopes := []string{"status:read", "plugins:read", "plugins:write", "runs:read", "runs:write", "schedules:read", "schedules:write", "logs:read", "users:write", "tokens:write"}
	scopeArray := map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": scopes}, "minItems": 1, "uniqueItems": true}
	schemas := map[string]any{
		"Error":          objectSchema(map[string]any{"error": objectSchema(map[string]any{"code": text, "message": text}, "code", "message")}, "error"),
		"User":           user,
		"Login":          objectSchema(map[string]any{"username": username, "password": map[string]any{"type": "string", "maxLength": 1024, "writeOnly": true}}, "username", "password"),
		"Auth":           objectSchema(map[string]any{"user": schemaRef("User"), "csrf_token": text}, "user", "csrf_token"),
		"PasswordChange": objectSchema(map[string]any{"current_password": map[string]any{"type": "string", "maxLength": 1024, "writeOnly": true}, "new_password": password}, "current_password", "new_password"),
		"UserCreate":     objectSchema(map[string]any{"username": username, "password": password, "role": role}, "username", "password", "role"),
		"UserUpdate":     objectSchema(map[string]any{"username": username, "password": password, "role": role, "is_active": boolean}, "username", "role", "is_active"),
		"TokenCreate":    objectSchema(map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}, "scopes": scopeArray, "expires_at": date}, "name", "scopes", "expires_at"),
		"TokenMetadata":  objectSchema(map[string]any{"id": text, "name": text, "scopes": scopeArray, "expires_at": date, "created_at": date}, "id", "name", "scopes", "expires_at", "created_at"),
		"TokenCreated":   objectSchema(map[string]any{"token": text, "metadata": schemaRef("TokenMetadata")}, "token", "metadata"),
		"UserList":       objectSchema(map[string]any{"items": arraySchema(schemaRef("User")), "total": integer}, "items", "total"),
		"TokenList":      objectSchema(map[string]any{"items": arraySchema(schemaRef("TokenMetadata")), "total": integer}, "items", "total"),
		"Status":         objectSchema(map[string]any{"version": text, "started_at": date, "plugins_active": integer, "runs_queued": integer, "runs_running": integer, "runs_succeeded": integer, "runs_failed": integer}, "version", "started_at", "plugins_active", "runs_queued", "runs_running", "runs_succeeded", "runs_failed"),
		"Health":         objectSchema(map[string]any{"status": map[string]any{"type": "string", "const": "ok"}}, "status"),
	}
	logProperties := map[string]any{}
	logFields := []string{"timestamp", "level", "message", "component", "plugin_id", "task_id", "run_id", "user_id", "request_id"}
	for _, field := range logFields {
		logProperties[field] = text
	}
	logProperties["timestamp"] = date
	schemas["Log"] = objectSchema(logProperties, logFields...)
	schemas["LogList"] = objectSchema(map[string]any{"items": arraySchema(schemaRef("Log")), "total": integer}, "items", "total")
	operations := []apiOperation{{"/auth/login", "post", "Login", "Auth", "200", "", ""}, {"/auth/me", "get", "", "Auth", "200", "viewer", ""}, {"/auth/logout", "post", "", "", "204", "viewer", ""}, {"/auth/password", "put", "PasswordChange", "", "204", "viewer", "users:write"}, {"/users", "get", "", "UserList", "200", "admin", "users:write"}, {"/users", "post", "UserCreate", "User", "201", "admin", "users:write"}, {"/users/{id}", "put", "UserUpdate", "User", "200", "admin", "users:write"}, {"/users/{id}", "delete", "", "", "204", "admin", "users:write"}, {"/tokens", "get", "", "TokenList", "200", "admin", "tokens:write"}, {"/tokens", "post", "TokenCreate", "TokenCreated", "201", "admin", "tokens:write"}, {"/tokens/{id}", "delete", "", "", "204", "admin", "tokens:write"}, {"/status", "get", "", "Status", "200", "viewer", "status:read"}, {"/logs", "get", "", "LogList", "200", "operator", "logs:read"}, {"/events", "get", "", "", "200", "viewer", "status:read"}, {"/openapi.json", "get", "", "", "200", "viewer", ""}, {"/healthz", "get", "", "Health", "200", "", ""}}
	paths := map[string]any{}
	for _, definition := range operations {
		success := map[string]any{"description": "Success"}
		if definition.response != "" {
			success["content"] = map[string]any{"application/json": map[string]any{"schema": schemaRef(definition.response)}}
		}
		responses := map[string]any{definition.status: success}
		for _, code := range []string{"400", "401", "403", "404", "409", "413", "415", "429", "500"} {
			responses[code] = map[string]any{"description": "API error", "content": map[string]any{"application/json": map[string]any{"schema": schemaRef("Error")}}}
		}
		operation := map[string]any{"responses": responses, "x-minimum-role": definition.role, "x-required-scope": definition.scope}
		if definition.role == "" {
			operation["security"] = []any{}
		}
		if definition.path == "/healthz" {
			operation["servers"] = []map[string]string{{"url": "/"}}
		}
		if definition.body != "" {
			operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schemaRef(definition.body)}}}
		}
		parameters := []any{}
		if strings.Contains(definition.path, "{id}") {
			parameters = append(parameters, map[string]any{"name": "id", "in": "path", "required": true, "schema": text})
		}
		if definition.method == "get" && (definition.path == "/users" || definition.path == "/tokens" || definition.path == "/logs") {
			parameters = append(parameters, map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "default": 50, "minimum": 1, "maximum": 200}}, map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "default": 0, "minimum": 0}})
		}
		if definition.path == "/logs" {
			for _, name := range []string{"level", "q", "run_id"} {
				parameters = append(parameters, map[string]any{"name": name, "in": "query", "schema": text})
			}
		}
		if definition.method != "get" {
			parameters = append(parameters, map[string]any{"name": "Origin", "in": "header", "description": "Required for login and session-authenticated mutations", "schema": text}, map[string]any{"name": "X-CSRF-Token", "in": "header", "description": "Required for session-authenticated mutations except login", "schema": text})
		}
		if len(parameters) > 0 {
			operation["parameters"] = parameters
		}
		if definition.path == "/events" {
			success["content"] = map[string]any{"text/event-stream": map[string]any{"schema": text}}
		}
		if definition.path == "/openapi.json" {
			success["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}
		}
		entry, ok := paths[definition.path].(map[string]any)
		if !ok {
			entry = map[string]any{}
			paths[definition.path] = entry
		}
		entry[definition.method] = operation
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "ModuDock Go API", "version": s.options.Version}, "servers": []map[string]string{{"url": "/api/v1"}}, "paths": paths, "security": []map[string][]string{{"session": {}}, {"bearer": {}}}, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"session": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName}, "bearer": map[string]any{"type": "http", "scheme": "bearer"}}}}
}
