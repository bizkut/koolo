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
		// Skip protected charms from being moved
		if isProtectedCharm(sc.Item) {
			if !sc.InStash {
				// Protected charms in inventory stay there
				inventoryCharms = append(inventoryCharms, sc)
			}
			continue
		}

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

		// Step 1: Open stash
		if !ctx.Data.OpenMenus.Stash {
			if err := OpenStash(); err != nil {
				ctx.Logger.Error(fmt.Sprintf("CharmManager: Failed to open stash: %v", err))
				return err
			}
			utils.Sleep(300)
		}

		// Step 2: Move inventory charm to stash (Ctrl+Click)
		SwitchStashTab(swap.FromStash.StashTab + 1)
		utils.Sleep(200)

		invScreenPos := ui.GetScreenCoordsForItem(swap.FromInventory.Item)
		ctx.HID.ClickWithModifier(game.LeftButton, invScreenPos.X, invScreenPos.Y, game.CtrlKey)
		utils.Sleep(300)
		ctx.RefreshGameData()

		// Step 3: Move stash charm to inventory (Ctrl+Click)
		stashScreenPos := ui.GetScreenCoordsForItem(swap.FromStash.Item)
		ctx.HID.ClickWithModifier(game.LeftButton, stashScreenPos.X, stashScreenPos.Y, game.CtrlKey)
		utils.Sleep(300)
		ctx.RefreshGameData()
	}

	// Close stash when done
	step.CloseAllMenus()

	return nil
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

// getCharmsInInventory returns all charm items currently in the player's inventory
func getCharmsInInventory() []data.Item {
	ctx := context.Get()
	charms := make([]data.Item, 0)

	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
		itemType := itm.Desc().Type
		if itemType == CharmTypeSmall || itemType == CharmTypeLarge || itemType == CharmTypeGrand {
			if itm.Identified {
				charms = append(charms, itm)
			}
		}
	}

	return charms
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

// dropCharm drops a charm from the inventory
func dropCharm(charm data.Item) error {
	ctx := context.Get()

	if !ctx.Data.OpenMenus.Inventory {
		ctx.HID.PressKeyBinding(ctx.Data.KeyBindings.Inventory)
		utils.Sleep(300)
		*ctx.Data = ctx.GameReader.GetData()
	}

	screenPos := ui.GetScreenCoordsForItem(charm)
	ctx.HID.Click(game.LeftButton, screenPos.X, screenPos.Y)
	utils.Sleep(200)

	*ctx.Data = ctx.GameReader.GetData()

	if len(ctx.Data.Inventory.ByLocation(item.LocationCursor)) == 0 {
		return fmt.Errorf("failed to pick up charm %s", getCharmName(charm))
	}

	DropMouseItem()

	*ctx.Data = ctx.GameReader.GetData()

	if len(ctx.Data.Inventory.ByLocation(item.LocationCursor)) > 0 {
		return fmt.Errorf("charm still on cursor after drop attempt")
	}

	return nil
}

// getCharmName returns a display name for the charm
func getCharmName(charm data.Item) string {
	if charm.IdentifiedName != "" {
		return charm.IdentifiedName
	}
	return string(charm.Name)
}
