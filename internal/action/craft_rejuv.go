package action

import (
	"errors"
	"log/slog"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/item"
	"github.com/hectorgimenez/d2go/pkg/data/npc"
	"github.com/hectorgimenez/koolo/internal/action/step"
	"github.com/hectorgimenez/koolo/internal/context"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/town"
	"github.com/hectorgimenez/koolo/internal/ui"
	"github.com/hectorgimenez/koolo/internal/utils"
	"github.com/lxn/win"
)

// Cheap potions that can be used for crafting (minor and light only)
var cheapHealingPotions = []item.Name{
	"MinorHealingPotion",
	"LightHealingPotion",
}

var cheapManaPotions = []item.Name{
	"MinorManaPotion",
	"LightManaPotion",
}

// Normal gems for Full Rejuv crafting
var normalGems = []item.Name{
	"Amethyst",
	"Diamond",
	"Emerald",
	"Ruby",
	"Sapphire",
	"Skull",
	"Topaz",
}

// Chipped gems for regular Rejuv crafting
var chippedGems = []item.Name{
	"ChippedAmethyst",
	"ChippedDiamond",
	"ChippedEmerald",
	"ChippedRuby",
	"ChippedSapphire",
	"ChippedSkull",
	"ChippedTopaz",
}

const (
	minGoldForCrafting = 5000
	potionsPerCraft    = 3   // 3 HP + 3 MP per rejuv
	goldPerCraft       = 500 // Safety buffer for buying potions (typ. <200g)
)

// CraftRejuvenationPotions crafts rejuv potions using cube recipes
func CraftRejuvenationPotions() error {
	ctx := context.Get()
	ctx.SetLastAction("CraftRejuvenationPotions")
	ctx.RefreshGameData()

	// Check if any crafting is enabled
	if !ctx.CharacterCfg.CubeRecipes.EnableFullRejuvCrafting && !ctx.CharacterCfg.CubeRecipes.EnableRejuvCrafting {
		return nil
	}

	// Check gold threshold
	if ctx.Data.PlayerUnit.TotalPlayerGold() < minGoldForCrafting {
		ctx.Logger.Debug("Not enough gold for rejuv crafting", slog.Int("gold", ctx.Data.PlayerUnit.TotalPlayerGold()))
		return nil
	}

	// Check if we have the Horadric Cube
	if _, found := ctx.Data.Inventory.Find("HoradricCube", item.LocationInventory, item.LocationStash, item.LocationSharedStash); !found {
		ctx.Logger.Debug("Horadric Cube not found, skipping rejuv crafting")
		return nil
	}

	// Calculate how many rejuvs we need
	targetCount := ctx.CharacterCfg.Inventory.RejuvPotionCount
	currentCount := countCurrentRejuvs(ctx)
	needed := targetCount - currentCount

	if needed <= 0 {
		ctx.Logger.Debug("Already have enough rejuv potions", slog.Int("current", currentCount), slog.Int("target", targetCount))
		return nil
	}

	ctx.Logger.Info("Crafting rejuvenation potions", slog.Int("needed", needed), slog.Int("current", currentCount))

	crafted := 0

	// Priority 1: Craft Full Rejuvs with normal gems
	if ctx.CharacterCfg.CubeRecipes.EnableFullRejuvCrafting {
		normalGemsAvailable := getAvailableGems(ctx, normalGems)
		for _, gem := range normalGemsAvailable {
			if crafted >= needed {
				break
			}
			// Check gold before each craft
			if ctx.Data.PlayerUnit.TotalPlayerGold() < goldPerCraft {
				ctx.Logger.Debug("Not enough gold to buy potions for crafting")
				break
			}
			if err := craftSingleRejuv(ctx, gem, true); err != nil {
				ctx.Logger.Warn("Failed to craft full rejuv", slog.String("error", err.Error()))
				continue
			}
			crafted++
			ctx.RefreshGameData() // Refresh to get updated counts
			ctx.Logger.Info("Crafted Full Rejuvenation Potion", slog.Int("crafted", crafted), slog.Int("needed", needed))
		}
	}

	// Priority 2: Craft regular Rejuvs with chipped gems
	if ctx.CharacterCfg.CubeRecipes.EnableRejuvCrafting && crafted < needed {
		chippedGemsAvailable := getAvailableGems(ctx, chippedGems)
		for _, gem := range chippedGemsAvailable {
			if crafted >= needed {
				break
			}
			// Check gold before each craft
			if ctx.Data.PlayerUnit.TotalPlayerGold() < goldPerCraft {
				ctx.Logger.Debug("Not enough gold to buy potions for crafting")
				break
			}
			if err := craftSingleRejuv(ctx, gem, false); err != nil {
				ctx.Logger.Warn("Failed to craft rejuv", slog.String("error", err.Error()))
				continue
			}
			crafted++
			ctx.RefreshGameData() // Refresh to get updated counts
			ctx.Logger.Info("Crafted Rejuvenation Potion", slog.Int("crafted", crafted), slog.Int("needed", needed))
		}
	}

	if crafted > 0 {
		ctx.Logger.Info("Finished crafting rejuvenation potions", slog.Int("total_crafted", crafted))
	}

	return nil
}

func countCurrentRejuvs(ctx *context.Status) int {
	count := 0
	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
		if itm.IsRejuvPotion() {
			count++
		}
	}
	// Also count in belt
	for _, itm := range ctx.Data.Inventory.Belt.Items {
		if itm.IsRejuvPotion() {
			count++
		}
	}
	return count
}

func getAvailableGems(ctx *context.Status, gemTypes []item.Name) []data.Item {
	var gems []data.Item

	// Check inventory first
	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
		for _, gemType := range gemTypes {
			if itm.Name == gemType {
				gems = append(gems, itm)
				break
			}
		}
	}

	// Then check stash
	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationStash, item.LocationSharedStash) {
		for _, gemType := range gemTypes {
			if itm.Name == gemType {
				gems = append(gems, itm)
				break
			}
		}
	}

	return gems
}

func craftSingleRejuv(ctx *context.Status, gem data.Item, isFullRejuv bool) error {
	potionType := "Rejuvenation"
	if isFullRejuv {
		potionType = "Full Rejuvenation"
	}
	ctx.Logger.Debug("Attempting to craft "+potionType+" Potion", slog.String("gem", string(gem.Name)))

	// Get cheap potions from inventory or buy them
	hpPotions, mpPotions, err := getCheapPotionsForCrafting(ctx)
	if err != nil {
		return err
	}

	if len(hpPotions) < potionsPerCraft || len(mpPotions) < potionsPerCraft {
		// Need to buy more potions
		hpNeeded := potionsPerCraft - len(hpPotions)
		mpNeeded := potionsPerCraft - len(mpPotions)

		if err := buyPotionsForCrafting(ctx, hpNeeded, mpNeeded); err != nil {
			return err
		}

		// Refresh potion list
		ctx.RefreshGameData()
		hpPotions, mpPotions, err = getCheapPotionsForCrafting(ctx)
		if err != nil {
			return err
		}

		if len(hpPotions) < potionsPerCraft || len(mpPotions) < potionsPerCraft {
			return errNotEnoughPotions
		}
	}

	// Prepare items for cube: 1 gem + 3 HP + 3 MP
	var itemsForCube []data.Item
	itemsForCube = append(itemsForCube, gem)
	itemsForCube = append(itemsForCube, hpPotions[:potionsPerCraft]...)
	itemsForCube = append(itemsForCube, mpPotions[:potionsPerCraft]...)

	// Add items to cube and transmute
	if err := CubeAddItems(itemsForCube...); err != nil {
		return err
	}

	return CubeTransmute()
}

var errNotEnoughPotions = errors.New("not enough cheap potions for crafting")

func getCheapPotionsForCrafting(ctx *context.Status) ([]data.Item, []data.Item, error) {
	var hpPotions, mpPotions []data.Item

	for _, itm := range ctx.Data.Inventory.ByLocation(item.LocationInventory) {
		// Check if it's a cheap healing potion
		for _, cheapHP := range cheapHealingPotions {
			if itm.Name == cheapHP {
				hpPotions = append(hpPotions, itm)
				break
			}
		}
		// Check if it's a cheap mana potion
		for _, cheapMP := range cheapManaPotions {
			if itm.Name == cheapMP {
				mpPotions = append(mpPotions, itm)
				break
			}
		}
	}

	return hpPotions, mpPotions, nil
}

func buyPotionsForCrafting(ctx *context.Status, hpNeeded, mpNeeded int) error {
	if hpNeeded <= 0 && mpNeeded <= 0 {
		return nil
	}

	ctx.Logger.Debug("Buying potions for crafting", slog.Int("hp_needed", hpNeeded), slog.Int("mp_needed", mpNeeded))

	// Get the vendor NPC for current town
	currentTown := town.GetTownByArea(ctx.Data.PlayerUnit.Area)
	if currentTown == nil {
		return errors.New("not in a town area, cannot buy potions")
	}
	vendorNPC := currentTown.RefillNPC()

	// Interact with vendor
	if err := InteractNPC(vendorNPC); err != nil {
		return err
	}

	// Open trade menu
	if vendorNPC == npc.Jamella {
		ctx.HID.KeySequence(win.VK_HOME, win.VK_RETURN)
	} else {
		ctx.HID.KeySequence(win.VK_HOME, win.VK_DOWN, win.VK_RETURN)
	}
	utils.Sleep(500)

	// Switch to potions tab (tab 4)
	SwitchVendorTab(4)
	ctx.RefreshGameData()

	// Find and buy Minor healing potions only (cheapest)
	if hpNeeded > 0 {
		if itm, found := ctx.Data.Inventory.Find("MinorHealingPotion", item.LocationVendor); found {
			buyPotionFromVendor(ctx, itm, hpNeeded)
		} else {
			ctx.Logger.Warn("Minor Healing Potion not found at vendor")
		}
	}

	// Find and buy Minor mana potions only (cheapest)
	if mpNeeded > 0 {
		if itm, found := ctx.Data.Inventory.Find("MinorManaPotion", item.LocationVendor); found {
			buyPotionFromVendor(ctx, itm, mpNeeded)
		} else {
			ctx.Logger.Warn("Minor Mana Potion not found at vendor")
		}
	}

	return step.CloseAllMenus()
}

func buyPotionFromVendor(ctx *context.Status, potion data.Item, quantity int) {
	screenPos := ui.GetScreenCoordsForItem(potion)

	for i := 0; i < quantity; i++ {
		ctx.HID.Click(game.RightButton, screenPos.X, screenPos.Y)
		utils.Sleep(300)
	}
}
