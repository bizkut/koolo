package action

import (
	"fmt"
	"sort"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/item"
	"github.com/hectorgimenez/d2go/pkg/data/stat"
	"github.com/hectorgimenez/d2go/pkg/nip"
	"github.com/hectorgimenez/koolo/internal/action/step"
	"github.com/hectorgimenez/koolo/internal/context"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/ui"
	"github.com/hectorgimenez/koolo/internal/utils"
)

// CharmScore represents a charm and its calculated score
type CharmScore struct {
	Item     data.Item
	Score    float64
	InStash  bool // true if charm is in stash, false if in inventory
	StashTab int  // which stash tab (0-indexed) if in stash
}

// Charm type constants
const (
	CharmTypeSmall = "scha" // Small Charm
	CharmTypeLarge = "lcha" // Large Charm
	CharmTypeGrand = "gcha" // Grand Charm
)

// Unique charm names that should always be kept
var uniqueCharms = []item.Name{
	"Annihilus",
	"Hellfire Torch",
	"Gheed's Fortune",
}

// ManageCharms optimizes charms between inventory and stash
// This should be called periodically, ideally during town visits
func ManageCharms() error {
	ctx := context.Get()

	// Check if charm manager is enabled
	if !ctx.CharacterCfg.CharmManager.Enabled {
		return nil
	}

	// Only manage charms in town for safety
	if !ctx.Data.PlayerUnit.Area.IsTown() {
		return nil
	}

	return OptimizeCharms()
}

// OptimizeCharms compares inventory and stash charms, swapping to maximize equipped charm power
func OptimizeCharms() error {
	ctx := context.Get()
	ctx.Logger.Info("CharmManager: Running full charm optimization...")

	// Get all charms from both inventory and stash
	allCharms := getAllCharms()
	if len(allCharms) == 0 {
		ctx.Logger.Debug("CharmManager: No charms found anywhere")
		return nil
	}

	ctx.Logger.Debug(fmt.Sprintf("CharmManager: Found %d total charms (inventory + stash)", len(allCharms)))

	// Score and sort all charms (highest first)
	sort.Slice(allCharms, func(i, j int) bool {
		return allCharms[i].Score > allCharms[j].Score
	})

	// Log top charms for debugging
	for i, sc := range allCharms {
		if i >= 5 {
			break
		}
		loc := "inventory"
		if sc.InStash {
			loc = fmt.Sprintf("stash tab %d", sc.StashTab+1)
		}
		ctx.Logger.Debug(fmt.Sprintf("CharmManager: #%d %s (%.1f) in %s", i+1, getCharmName(sc.Item), sc.Score, loc))
	}

	// Identify charms that should be swapped
	// We want high-score stash charms to replace low-score inventory charms
	inventoryCharms := make([]CharmScore, 0)
	stashCharms := make([]CharmScore, 0)

	for _, sc := range allCharms {
		if sc.InStash {
			stashCharms = append(stashCharms, sc)
		} else {
			inventoryCharms = append(inventoryCharms, sc)
		}
	}

	// Find swaps: stash charm better than inventory charm
	swapsNeeded := findCharmSwaps(inventoryCharms, stashCharms)
	if len(swapsNeeded) == 0 {
		ctx.Logger.Info("CharmManager: No beneficial swaps found")
		return nil
	}

	ctx.Logger.Info(fmt.Sprintf("CharmManager: Found %d beneficial swaps", len(swapsNeeded)))

	// Execute swaps
	return executeCharmSwaps(swapsNeeded)
}

// CharmSwap represents a swap operation
type CharmSwap struct {
	FromInventory CharmScore // Charm to move from inventory to stash
	FromStash     CharmScore // Charm to move from stash to inventory
}

// findCharmSwaps identifies which charms should be swapped
func findCharmSwaps(inventoryCharms, stashCharms []CharmScore) []CharmSwap {
	ctx := context.Get()
	swaps := make([]CharmSwap, 0)

	// For each stash charm, see if it's better than any inventory charm of same size
	for _, stashCharm := range stashCharms {
		stashType := stashCharm.Item.Desc().Type

		// Find the worst inventory charm of the same type
		worstIdx := -1
		worstScore := stashCharm.Score // Must be better than stash charm

		for i, invCharm := range inventoryCharms {
			// Skip if already used in a swap
			if invCharm.Score < 0 {
				continue
			}
			// Skip locked slots
			if IsInLockedInventorySlot(invCharm.Item) {
				continue
			}
			// Skip protected charms (Skillers/Uniques) - never swap them out
			if isProtectedCharm(invCharm.Item) {
				continue
			}
			// Must be same charm type (size)
			if invCharm.Item.Desc().Type != stashType {
				continue
			}
			// Must be worse than the stash charm
			if invCharm.Score < worstScore {
				worstScore = invCharm.Score
				worstIdx = i
			}
		}

		if worstIdx >= 0 {
			swap := CharmSwap{
				FromInventory: inventoryCharms[worstIdx],
				FromStash:     stashCharm,
			}
			swaps = append(swaps, swap)
			// Mark as used
			inventoryCharms[worstIdx].Score = -1
			ctx.Logger.Debug(fmt.Sprintf("CharmManager: Will swap %s (%.1f) with %s (%.1f)",
				getCharmName(swap.FromInventory.Item), swap.FromInventory.Score,
				getCharmName(swap.FromStash.Item), swap.FromStash.Score))
		}
	}

	return swaps
}

// executeCharmSwaps performs the actual item movements
func executeCharmSwaps(swaps []CharmSwap) error {
	ctx := context.Get()

	for _, swap := range swaps {
		ctx.Logger.Info(fmt.Sprintf("CharmManager: Swapping %s (%.1f) for %s (%.1f)",
			getCharmName(swap.FromInventory.Item), swap.FromInventory.Score,
			getCharmName(swap.FromStash.Item), swap.FromStash.Score))

		// Safety: Clear cursor before starting swap
		if cleared, dropped := clearCursorSafely(); !cleared {
			ctx.Logger.Error("CharmManager: Could not clear cursor, aborting swaps")
			step.CloseAllMenus()
			return fmt.Errorf("cursor not empty")
		} else if dropped {
			// If we dropped something, try to recover it once
			if recoverDroppedCharm() {
				// If we recovered it, we are holding it again. Try to stash it one last time?
				// Or just return error to stop swapping and let standard cleanup handle it?
				// Safest is to error out so we don't loop.
				ctx.Logger.Warn("CharmManager: Recovered dropped charm, aborting swap to prevent loops")
				return fmt.Errorf("recovered dropped item")
			}
		}

		// Step 1: Open stash if not open
		if !ctx.Data.OpenMenus.Stash {
			if err := OpenStash(); err != nil {
				ctx.Logger.Error(fmt.Sprintf("CharmManager: Failed to open stash: %v", err))
				return err
			}
			utils.Sleep(300)
			ctx.RefreshGameData()
		}

		// Step 2: Switch to the correct stash tab
		SwitchStashTab(swap.FromStash.StashTab + 1)
		utils.Sleep(200)

		// Step 3: Move inventory charm to stash (Ctrl+Click)
		// Re-find the item in current inventory data to get fresh coordinates
		var invItem data.Item
		var foundInv bool
		for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
			if itm.UnitID == swap.FromInventory.Item.UnitID {
				invItem = itm
				foundInv = true
				break
			}
		}
		if !foundInv {
			ctx.Logger.Warn(fmt.Sprintf("CharmManager: Inventory charm %s no longer found, skipping swap", getCharmName(swap.FromInventory.Item)))
			continue
		}

		invScreenPos := ui.GetScreenCoordsForItem(invItem)
		ctx.HID.ClickWithModifier(game.LeftButton, invScreenPos.X, invScreenPos.Y, game.CtrlKey)
		utils.Sleep(300)
		ctx.RefreshGameData()

		// Safety: If item is on cursor (stash full?), put it back in inventory
		if len(ctx.Data.Inventory.ByLocation(item.LocationCursor)) > 0 {
			ctx.Logger.Warn("CharmManager: Item stuck on cursor after stash attempt, returning to inventory")
			ctx.HID.Click(game.LeftButton, invScreenPos.X, invScreenPos.Y)
			utils.Sleep(300)
			ctx.RefreshGameData()
			continue
		}

		// Verify inventory charm moved to stash
		stillInInventory := false
		for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
			if itm.UnitID == swap.FromInventory.Item.UnitID {
				stillInInventory = true
				break
			}
		}
		if stillInInventory {
			ctx.Logger.Warn(fmt.Sprintf("CharmManager: Failed to move %s to stash, skipping this swap", getCharmName(swap.FromInventory.Item)))
			continue
		}

		// Step 4: Move stash charm to inventory (Ctrl+Click)
		// Re-find the item in current stash data to get fresh coordinates
		var stashItem data.Item
		var foundStash bool
		for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationStash, item.LocationSharedStash) {
			if itm.UnitID == swap.FromStash.Item.UnitID {
				stashItem = itm
				foundStash = true
				break
			}
		}
		if !foundStash {
			ctx.Logger.Warn(fmt.Sprintf("CharmManager: Stash charm %s no longer found, swap incomplete", getCharmName(swap.FromStash.Item)))
			continue
		}

		stashScreenPos := ui.GetScreenCoordsForItem(stashItem)
		ctx.HID.ClickWithModifier(game.LeftButton, stashScreenPos.X, stashScreenPos.Y, game.CtrlKey)
		utils.Sleep(300)
		ctx.RefreshGameData()

		// Safety: If item is on cursor (inventory full?), put it back in stash
		if len(ctx.Data.Inventory.ByLocation(item.LocationCursor)) > 0 {
			ctx.Logger.Warn("CharmManager: Item stuck on cursor after inventory attempt, returning to stash")
			ctx.HID.Click(game.LeftButton, stashScreenPos.X, stashScreenPos.Y)
			utils.Sleep(300)
			ctx.RefreshGameData()
			continue
		}

		// Verify stash charm moved to inventory
		nowInInventory := false
		for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
			if itm.UnitID == swap.FromStash.Item.UnitID {
				nowInInventory = true
				break
			}
		}
		if !nowInInventory {
			ctx.Logger.Warn(fmt.Sprintf("CharmManager: Failed to move %s to inventory", getCharmName(swap.FromStash.Item)))
		}
	}

	// Final safety: Ensure cursor is clear before closing
	if cleared, dropped := clearCursorSafely(); !cleared {
		ctx.Logger.Warn("CharmManager: Cursor not clear after swaps")
	} else if dropped {
		recoverDroppedCharm()
	}

	// Close stash when done
	step.CloseAllMenus()

	return nil
}

// clearCursorSafely ensures no item is on the cursor, with retry limit to prevent loops
// Returns (cleared status, whether item was dropped)
func clearCursorSafely() (bool, bool) {
	ctx := context.Get()
	const maxRetries = 3
	dropped := false

	for i := 0; i < maxRetries; i++ {
		ctx.RefreshGameData()
		cursorItems := ctx.Data.Inventory.ByLocation(item.LocationCursor)
		if len(cursorItems) == 0 {
			return true, dropped // Cursor is clear
		}

		ctx.Logger.Warn(fmt.Sprintf("CharmManager: Item on cursor, attempting to clear (attempt %d/%d)", i+1, maxRetries))

		// Try to drop the item safely
		DropMouseItem()
		dropped = true
		utils.Sleep(500)
	}

	ctx.RefreshGameData()
	return len(ctx.Data.Inventory.ByLocation(item.LocationCursor)) == 0, dropped
}

// recoverDroppedCharm attempts to pick up a charm that was accidentally dropped
func recoverDroppedCharm() bool {
	ctx := context.Get()
	ctx.RefreshGameData()

	// Find the closest charm on ground
	var closestCharm data.Item
	minDist := 999
	found := false

	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationGround) {
		if isCharmItem(itm) {
			dist := ctx.PathFinder.DistanceFromMe(itm.Position)
			if dist < 5 && dist < minDist {
				minDist = dist
				closestCharm = itm
				found = true
			}
		}
	}

	if found {
		ctx.Logger.Warn(fmt.Sprintf("CharmManager: Attempting to recover dropped charm: %s", getCharmName(closestCharm)))
		// Attempt pickup (max 3 tries)
		if err := step.PickupItem(closestCharm, 3); err == nil {
			ctx.Logger.Info("CharmManager: Successfully recovered dropped charm")
			return true
		} else {
			ctx.Logger.Error(fmt.Sprintf("CharmManager: Failed to recover dropped charm: %v", err))
		}
	}

	return false
}

// getAllCharms returns all charms from inventory and stash with scores
// Only includes charms that match pickit (NIP) rules
func getAllCharms() []CharmScore {
	ctx := context.Get()
	allCharms := make([]CharmScore, 0)

	// Get inventory charms
	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
		if !isCharmItem(itm) || !itm.Identified {
			continue
		}
		// Only evaluate charms that match pickit rules
		if _, res := ctx.CharacterCfg.Runtime.Rules.EvaluateAll(itm); res != nip.RuleResultFullMatch {
			continue
		}
		score := getCharmScore(itm)
		allCharms = append(allCharms, CharmScore{
			Item:    itm,
			Score:   score,
			InStash: false,
		})
	}

	// Get stash charms (all tabs)
	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationStash, item.LocationSharedStash) {
		if !isCharmItem(itm) || !itm.Identified {
			continue
		}
		// Only evaluate charms that match pickit rules
		if _, res := ctx.CharacterCfg.Runtime.Rules.EvaluateAll(itm); res != nip.RuleResultFullMatch {
			continue
		}
		score := getCharmScore(itm)
		allCharms = append(allCharms, CharmScore{
			Item:     itm,
			Score:    score,
			InStash:  true,
			StashTab: itm.Location.Page,
		})
	}

	return allCharms
}

// isCharmItem checks if an item is a charm
func isCharmItem(itm data.Item) bool {
	itemType := itm.Desc().Type
	return itemType == CharmTypeSmall || itemType == CharmTypeLarge || itemType == CharmTypeGrand
}

// getCharmScore calculates a score for a charm based on its stats
func getCharmScore(charm data.Item) float64 {
	score := 0.0

	if lifeStat, found := charm.FindStat(stat.MaxLife, 0); found {
		score += float64(lifeStat.Value) * 1.0
	}

	if manaStat, found := charm.FindStat(stat.MaxMana, 0); found {
		score += float64(manaStat.Value) * 0.5
	}

	fireRes := 0
	coldRes := 0
	lightRes := 0
	poisonRes := 0

	if fr, found := charm.FindStat(stat.FireResist, 0); found {
		fireRes = fr.Value
		score += float64(fr.Value) * 2.0
	}
	if cr, found := charm.FindStat(stat.ColdResist, 0); found {
		coldRes = cr.Value
		score += float64(cr.Value) * 2.0
	}
	if lr, found := charm.FindStat(stat.LightningResist, 0); found {
		lightRes = lr.Value
		score += float64(lr.Value) * 2.0
	}
	if pr, found := charm.FindStat(stat.PoisonResist, 0); found {
		poisonRes = pr.Value
		score += float64(pr.Value) * 1.0
	}

	if fireRes > 0 && coldRes > 0 && lightRes > 0 && poisonRes > 0 {
		minRes := min(fireRes, coldRes, lightRes, poisonRes)
		score += float64(minRes) * 2.0
	}

	if mfStat, found := charm.FindStat(stat.MagicFind, 0); found {
		score += float64(mfStat.Value) * 1.5
	}

	if gfStat, found := charm.FindStat(stat.GoldFind, 0); found {
		score += float64(gfStat.Value) * 0.5
	}

	if fhrStat, found := charm.FindStat(stat.FasterHitRecovery, 0); found {
		score += float64(fhrStat.Value) * 1.0
	}

	if frwStat, found := charm.FindStat(stat.FasterRunWalk, 0); found {
		score += float64(frwStat.Value) * 0.8
	}

	if arStat, found := charm.FindStat(stat.AttackRating, 0); found {
		score += float64(arStat.Value) * 0.05
	}

	if minDmg, found := charm.FindStat(stat.MinDamage, 0); found {
		score += float64(minDmg.Value) * 2.0
	}
	if maxDmg, found := charm.FindStat(stat.MaxDamage, 0); found {
		score += float64(maxDmg.Value) * 1.5
	}

	if strStat, found := charm.FindStat(stat.Strength, 0); found {
		score += float64(strStat.Value) * 2.0
	}
	if dexStat, found := charm.FindStat(stat.Dexterity, 0); found {
		score += float64(dexStat.Value) * 2.0
	}
	if vitStat, found := charm.FindStat(stat.Vitality, 0); found {
		score += float64(vitStat.Value) * 2.5
	}
	if eneStat, found := charm.FindStat(stat.Energy, 0); found {
		score += float64(eneStat.Value) * 1.0
	}

	for statID := stat.ID(188); statID <= stat.ID(250); statID++ {
		if skillStat, found := charm.FindStat(statID, 0); found && skillStat.Value > 0 {
			score += 50.0
			break
		}
	}

	return score
}

// isProtectedCharm checks if a charm should never be moved
func isProtectedCharm(charm data.Item) bool {
	ctx := context.Get()

	if ctx.CharacterCfg.CharmManager.KeepUniques {
		if charm.Quality == item.QualityUnique {
			return true
		}
		for _, uniqueName := range uniqueCharms {
			if charm.Name == uniqueName {
				return true
			}
		}
	}

	if ctx.CharacterCfg.CharmManager.KeepSkillers {
		if charm.Desc().Type == CharmTypeGrand {
			for statID := stat.ID(188); statID <= stat.ID(250); statID++ {
				if skillStat, found := charm.FindStat(statID, 0); found && skillStat.Value > 0 {
					return true
				}
			}
		}
	}

	return false
}

// getCharmName returns a display name for the charm
func getCharmName(charm data.Item) string {
	if charm.IdentifiedName != "" {
		return charm.IdentifiedName
	}
	return string(charm.Name)
}
