# meraki-entra-sync

This script keeps Microsoft Entra Conditional Access [named locations](https://learn.microsoft.com/en-us/entra/identity/conditional-access/concept-assignment-network) in sync with the public IP addresses of Cisco Meraki networks.

Every minute, the script fetches all Meraki networks carrying one of the configured tags, reads the public IP addresses (IPv4 and IPv6) of their activate appliance uplinks, and reconciles them against the tenant's named locations, matching by display name:

- **The location already matches** → nothing happens.
- **No named location with the network name exists** → new entry is created with the uplink IPs, marked as *trusted*
- **The location already exists but its IP ranges differ** → the ranges are updated. The trusted flag is deliberately left untouched, so unchecking *"Mark as trusted location"* in the Entra portal sticks

The tool is stateless: Entra is the single source of truth, and each run is a fresh comparison, so manual deletions or IP changes self-heal on the next tick. Conditional Access *policies* are never touched - locations created here take effect through whatever policies already reference trusted locations.

## Configuration

All configuration must be defined as environment variables (see `.env.sample`):

| Variable | Description |
| --- | --- |
| `AZURE_TENANT_ID` | Entra tenant ID |
| `AZURE_CLIENT_ID` | App registration used for Microsoft Graph (needs `Policy.Read.All` and `Policy.ReadWrite.ConditionalAccess`) |
| `AZURE_CLIENT_SECRET` | Client secret for the app registration process |
| `MERAKI_CLIENT_SECRET` | Meraki Dashboard API key (read access is sufficient) |
| `MERAKI_ORGANIZATION_ID` | Meraki organization to scan |
| `MERAKI_ALLOWED_NETWORK_TAGS` | Comma-separated list of network tags; only matching networks are synced |
