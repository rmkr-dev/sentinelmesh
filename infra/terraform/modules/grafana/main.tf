variable "name" { type = string }
variable "location" { type = string }
variable "resource_group_name" { type = string }
variable "tags" { type = map(string) }

resource "azurerm_dashboard_grafana" "this" {
  name                  = var.name
  resource_group_name   = var.resource_group_name
  location              = var.location
  grafana_major_version = "11"
  tags                  = var.tags

  identity {
    type = "SystemAssigned"
  }
}

output "endpoint" { value = azurerm_dashboard_grafana.this.endpoint }
output "id" { value = azurerm_dashboard_grafana.this.id }
