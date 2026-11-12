package mcpserver

// Instructions is sent to every client on initialize and says what an agent
// may do here: create drafts, nothing else.
const Instructions = "Kontor is a personal finance manager. You cannot create or change contracts, purchases, costs, vehicles, categories, accounts or transactions. " +
	"Create drafts instead; the user reviews them in the Kontor inbox and accepts or rejects them. " +
	"Amounts are EUR; dates are YYYY-MM-DD. " +
	"Use the list_* tools to get category and vehicle IDs. " +
	"Give every draft a short title and a stable `key` (for example the file name or a hash) so re-runs do not create duplicate drafts while one is still pending."
