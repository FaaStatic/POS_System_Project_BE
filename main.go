// Package main novi_shop API.
//
//	@title			novi_shop API
//	@version		1.0
//	@description	REST API backend for the novi_shop cashier/store application.
//	@host			localhost:3030
//	@BasePath		/
//	@schemes		http
//
//	@securityDefinitions.apikey	BearerAuth
//	@in								header
//	@name							Authorization
//	@description					Enter the token in the format: Bearer {access_token}
package main

import "shop_project_be/cmd"

func main() {
	cmd.Execute()
}
