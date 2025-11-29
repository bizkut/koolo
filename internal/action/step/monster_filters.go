package step

import (
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/koolo/internal/context"
	"github.com/hectorgimenez/koolo/internal/game"
)

func MonsterDefaultFilter() data.MonsterFilter {
	return func(monsters data.Monsters) (filteredMonsters []data.Monster) {
		for _, m := range monsters {
			if !ShouldIgnoreMonster(m, false) {
				filteredMonsters = append(filteredMonsters, m)
			}
		}
		return filteredMonsters
	}
}

func MonsterNavigationFilter() data.MonsterFilter {
	return func(monsters data.Monsters) (filteredMonsters []data.Monster) {
		for _, m := range monsters {
			if !ShouldIgnoreMonster(m, context.Get().CharacterCfg.Character.NavigationFocusElites) {
				filteredMonsters = append(filteredMonsters, m)
			}
		}
		return filteredMonsters
	}
}

func MonsterClearLevelFilter() data.MonsterFilter {
	return func(monsters data.Monsters) (filteredMonsters []data.Monster) {
		for _, m := range monsters {
			if !ShouldIgnoreMonster(m, context.Get().CharacterCfg.Character.ClearLevelFocusElites) {
				filteredMonsters = append(filteredMonsters, m)
			}
		}
		return filteredMonsters
	}
}

func ShouldIgnoreMonster(m data.Monster, focusElites bool) bool {
	ctx := context.Get()

	//Force fight mandatory enemies
	if game.IsQuestEnemy(m) {
		return false
	}

	//Immunity check if not blocked
	if !ctx.CurrentGame.IsBlocked() && len(ctx.CharacterCfg.Character.SkipOnImmunities) > 0 {
		ImmuneToAll := true
		for _, resist := range ctx.CharacterCfg.Character.SkipOnImmunities {
			if !m.IsImmune(resist) {
				ImmuneToAll = false
			}
		}
		if ImmuneToAll {
			return true
		}
	}

	if focusElites {
		if !m.IsElite() {
			return true
		}
	}

	return false
}
