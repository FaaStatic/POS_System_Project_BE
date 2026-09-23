package middleware

import "github.com/gofiber/fiber/v3/middleware/helmet"

func GetXSSConfig() helmet.Config {
	return helmet.Config{
		XSSProtection:             "1; mode=block",
		XFrameOptions:             "SAMEORIGIN",
		ContentTypeNosniff:        "nosniff",
		ReferrerPolicy:            "no-referrer",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		OriginAgentCluster:        "?1",
		XDNSPrefetchControl:       "off",
		XDownloadOptions:          "noopen",
		XPermittedCrossDomain:     "none",
		ContentSecurityPolicy:     "default-src 'self'; script-src 'self' 'unsafe-inline' https://unpkg.com; style-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https://unpkg.com; font-src 'self'; connect-src 'self'",
	}
}
