variable "name" { type = string }
variable "location" { type = string }
variable "resource_group_name" { type = string }
variable "tags" { type = map(string) }

data "azurerm_client_config" "current" {}

resource "azurerm_user_assigned_identity" "collector" {
  name                = var.name
  location            = var.location
  resource_group_name = var.resource_group_name
  tags                = var.tags
}

output "client_id" { value = azurerm_user_assigned_identity.collector.client_id }
output "principal_id" { value = azurerm_user_assigned_identity.collector.principal_id }
output "tenant_id" { value = data.azurerm_client_config.current.tenant_id }
output "id" { value = azurerm_user_assigned_identity.collector.id }
