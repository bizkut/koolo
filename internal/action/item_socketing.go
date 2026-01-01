package action

import (
	"fmt"
	"strings"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/item"
	"github.com/hectorgimenez/d2go/pkg/data/stat"
	"github.com/hectorgimenez/koolo/internal/context"
)

func ItemSocketing() error {
	ctx := context.Get()
	if !ctx.CharacterCfg.ItemSocketing.Enabled || len(ctx.CharacterCfg.ItemSocketing.Recipes) == 0 {
		return nil
	}

	ctx.SetLastAction("ItemSocketing")
	ctx.RefreshGameData()

	items := ctx.Data.Inventory.ByLocation(item.LocationStash, item.LocationSharedStash, item.LocationInventory)

	for _, recipe := range ctx.CharacterCfg.ItemSocketing.Recipes {
		// 1. Find the Base Item with Sockets
		var baseItem *data.Item
		for _, itm := range items {
			// Check if name matches (simplistic check, might want NIP later)
			// Using strings.Contains for flexibility (e.g. "Harlequin" matches "Harlequin Quest")
			// But for safety, exact match or specialized matching is better.
			// Let's assume standard item names for now.
			if matchesItemName(itm, recipe.ItemName) {
				// Check for open sockets
				sockets, found := itm.FindStat(stat.NumSockets, 0)
				if !found || sockets.Value == 0 {
					continue
				}

				// Optional: Check if sockets are already full?
				// The game data might not easily say "empty sockets" vs "filled sockets" directly without checking modifiers.
				// However, usually detailed item stats would show if it has gems.
				// For now, we rely on the fact that if we can't insert, it will fail gracefully or we check description.
				// Actually, simpler: Attempt to socket if we find one.

				// We need to verify it actually HAS empty sockets.
				// A simple heuristic: The number of "Socketed Items" stat vs "Sockets" stat?
				// D2Go might not expose 'numSocketedItems' directly on the item struct easily.
				// But generally, we only want to socket items that are purely base + sockets.
				// If it already has some gems, it's safer to skip or we need advanced logic.
				// For this V1, let's look for the base item.

				baseItem = &itm
				break
			}
		}

		if baseItem == nil {
			continue // Recipe base not found
		}

		// 2. Find the Ingredient (Gem/Rune)
		var ingredientItem *data.Item
		for _, itm := range items {
			// Skip the base item itself
			if itm.UnitID == baseItem.UnitID {
				continue
			}

			if matchesItemName(itm, recipe.SocketWithName) {
				ingredientItem = &itm
				break
			}
		}

		if ingredientItem == nil {
			continue // Ingredient not found
		}

		ctx.Logger.Info(fmt.Sprintf("Socketing %s into %s", ingredientItem.Name, baseItem.Name))

		// 3. Perform Socketing
		err := InsertIntoSockets([]data.Item{*ingredientItem}, *baseItem)
		if err != nil {
			ctx.Logger.Error(fmt.Sprintf("Failed to socket %s into %s: %v", ingredientItem.Name, baseItem.Name, err))
			continue
		}

		// If successful, we might want to return or continue.
		// Return to ensure we refresh data completely before next attempt.
		return nil
	}

	return nil
}

func matchesItemName(itm data.Item, targetName string) bool {
	// Basic clean up
	targetName = strings.TrimSpace(strings.ToLower(targetName))
	itemName := strings.TrimSpace(strings.ToLower(string(itm.Name)))
	itemDescName := strings.TrimSpace(strings.ToLower(itm.Desc().Name))

	if itemName == targetName || itemDescName == targetName {
		return true
	}

	// Check for "Perfect Topaz" vs "Topaz" mismatch if needed, but usually config should be precise.
	// Handling cases like "Harlequin Crest" which refers to "Shako" unique.
	// The user might put "Harlequin Crest" or "Shako".
	// If unit is unique, check local name.

	// Allow partial match for standard names if not found exactly?
	// No, exact match is safer for socketing.

	return false
}
