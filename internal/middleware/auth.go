package middleware

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	"github.com/sekolahpintar/dashboard-engine/internal/model"
)

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"success":false,"message":%q}`, msg)
}

type contextKey string

const claimsKey contextKey = "user_claims"

// Auth validates the Bearer JWT token and loads user claims into context.
func Auth(jwtSecret string, db *sqlx.DB) func(http.Handler) http.Handler {
	secretBytes := []byte(jwtSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				jsonErr(w, "Unauthenticated", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return secretBytes, nil
			}, jwt.WithExpirationRequired())

			if err != nil || !token.Valid {
				jsonErr(w, "Token invalid or expired", http.StatusUnauthorized)
				return
			}

			mapClaims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				jsonErr(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			var userID int64
			switch v := mapClaims["sub"].(type) {
			case string:
				fmt.Sscanf(v, "%d", &userID)
			case float64:
				userID = int64(v)
			}
			if userID == 0 {
				jsonErr(w, "Invalid token sub", http.StatusUnauthorized)
				return
			}

			claims, err := loadUserClaims(r.Context(), db, userID)
			if err != nil {
				jsonErr(w, "Unauthenticated", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClaims retrieves UserClaims from request context.
func GetClaims(ctx context.Context) *model.UserClaims {
	c, _ := ctx.Value(claimsKey).(*model.UserClaims)
	return c
}

func loadUserClaims(ctx context.Context, db *sqlx.DB, userID int64) (*model.UserClaims, error) {
	var isActive int8
	err := db.QueryRowContext(ctx,
		`SELECT is_active FROM sys_users WHERE id = $1 AND deleted_at IS NULL`,
		userID,
	).Scan(&isActive)
	if err == sql.ErrNoRows || isActive == 0 {
		return nil, fmt.Errorf("user not found or inactive")
	}
	if err != nil {
		return nil, err
	}

	// Load roles
	rows, err := db.QueryContext(ctx, `
		SELECT r.code
		FROM sys_roles r
		INNER JOIN sys_user_roles ur ON ur.sys_role_id = r.id
		WHERE ur.sys_user_id = $1
		  AND ur.deleted_at IS NULL
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		roles = append(roles, code)
	}

	claims := &model.UserClaims{UserID: userID, Roles: roles}

	// Optionally load siswa / guru IDs
	var siswaID int64
	err = db.QueryRowContext(ctx,
		`SELECT id FROM mst_siswa WHERE sys_user_id = $1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).Scan(&siswaID)
	if err == nil {
		claims.SiswaID = &siswaID
	}

	var guruID int64
	err = db.QueryRowContext(ctx,
		`SELECT id FROM mst_guru WHERE sys_user_id = $1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).Scan(&guruID)
	if err == nil {
		claims.GuruID = &guruID
	}

	var waliID int64
	err = db.QueryRowContext(ctx,
		`SELECT id FROM mst_wali WHERE sys_user_id = $1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).Scan(&waliID)
	if err == nil {
		claims.WaliID = &waliID
	}

	return claims, nil
}
