package provider

import (
	"errors"
	"net/http"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type connectionModel struct {
	ID                types.String `tfsdk:"id"`
	ConnectionID      types.String `tfsdk:"connection_id"`
	Provider          types.String `tfsdk:"provider_name"`
	ProviderAccountID types.String `tfsdk:"provider_account_id"`
	TenantID          types.String `tfsdk:"tenant_id"`
	DisplayName       types.String `tfsdk:"display_name"`
}

func connectionState(connection *costgraph.Connection) connectionModel {
	return connectionModel{
		ID: types.StringValue(connection.ID), ConnectionID: types.StringValue(connection.ConnectionID),
		Provider: types.StringValue(connection.Provider), ProviderAccountID: types.StringValue(connection.ProviderAccountID),
		TenantID: types.StringValue(connection.TenantID), DisplayName: types.StringValue(connection.DisplayName),
	}
}

func isNotFound(err error) bool {
	var apiError *costgraph.APIError
	return errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound
}
