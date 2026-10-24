package middleware

import "github.com/gofiber/fiber/v3/middleware/cors"

// GetCorsConfig: tanpa cmsOrigins API terbuka untuk semua origin tapi tanpa
// credentials, jadi cookie refresh CMS tidak bisa dipakai. Isi server.cms_origins
// dengan origin CMS (mis. https://cms.tokoku.id) supaya browser mengirim cookie.
func GetCorsConfig(cmsOrigins []string) cors.Config {
	cfg := cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Client"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
	}
	if len(cmsOrigins) > 0 {
		cfg.AllowOrigins = cmsOrigins
		cfg.AllowCredentials = true
	}
	return cfg
}
