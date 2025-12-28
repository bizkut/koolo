package pather

import (
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/d2go/pkg/data/object"
	"github.com/hectorgimenez/d2go/pkg/data/skill"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/utils"
)

func (pf *PathFinder) RandomMovement() {
	// First try directional movement to find a walkable position
	if pf.DirectionalMovement() {
		return
	}

	// Fallback to pure random if directional failed
	midGameX := pf.gr.GameAreaSizeX / 2
	midGameY := pf.gr.GameAreaSizeY / 2
	x := midGameX + rand.Intn(midGameX) - (midGameX / 2)
	y := midGameY + rand.Intn(midGameY) - (midGameY / 2)
	pf.hid.MovePointer(x, y)
	// Use appropriate movement method for town vs field
	if pf.data.PlayerUnit.Area.IsTown() {
		pf.hid.Click(game.LeftButton, x, y)
	} else {
		pf.pressForceMove(x, y)
	}
	utils.Sleep(100)
}

// DirectionalMovement tries to move in 8 cardinal directions, preferring walkable positions
// Returns true if a movement was attempted
func (pf *PathFinder) DirectionalMovement() bool {
	// Safety check for nil AreaData or Grid
	if pf.data.AreaData.Grid == nil || pf.data.AreaData.Grid.CollisionGrid == nil {
		return false
	}

	currentPos := pf.data.PlayerUnit.Position

	// 8 directions: N, NE, E, SE, S, SW, W, NW (distance of 5 units)
	directions := []struct{ dx, dy int }{
		{0, -5}, {5, -5}, {5, 0}, {5, 5},
		{0, 5}, {-5, 5}, {-5, 0}, {-5, -5},
	}

	// Shuffle directions to avoid always trying the same order
	rand.Shuffle(len(directions), func(i, j int) {
		directions[i], directions[j] = directions[j], directions[i]
	})

	// Helper function to attempt movement to a position
	attemptMove := func(targetPos data.Position, checkWalkable bool) bool {
		// Check walkability if required
		if checkWalkable && !pf.data.AreaData.IsWalkable(targetPos) {
			return false
		}

		// Convert to screen coordinates
		screenX, screenY := pf.GameCoordsToScreenCords(targetPos.X, targetPos.Y)

		// Bounds check for screen coordinates
		if screenX <= 50 || screenX >= pf.gr.GameAreaSizeX-50 ||
			screenY <= 50 || screenY >= int(float32(pf.gr.GameAreaSizeY)/1.19) {
			return false
		}

		// Perform the movement
		pf.hid.MovePointer(screenX, screenY)
		if pf.data.PlayerUnit.Area.IsTown() {
			pf.hid.Click(game.LeftButton, screenX, screenY)
		} else {
			pf.pressForceMove(screenX, screenY)
		}
		utils.Sleep(150)
		return true
	}

	// First pass: try each direction with walkability check
	for _, dir := range directions {
		targetPos := data.Position{
			X: currentPos.X + dir.dx,
			Y: currentPos.Y + dir.dy,
		}
		if attemptMove(targetPos, true) {
			return true
		}
	}

	// Second pass: try larger radius (10 units) with walkability check
	for _, dir := range directions {
		targetPos := data.Position{
			X: currentPos.X + dir.dx*2,
			Y: currentPos.Y + dir.dy*2,
		}
		if attemptMove(targetPos, true) {
			return true
		}
	}

	// Last resort: try any direction without walkability check (maybe we're on a bad tile)
	for _, dir := range directions {
		targetPos := data.Position{
			X: currentPos.X + dir.dx,
			Y: currentPos.Y + dir.dy,
		}
		if attemptMove(targetPos, false) {
			return true
		}
	}

	return false
}

func (pf *PathFinder) DistanceFromMe(p data.Position) int {
	return DistanceFromPoint(pf.data.PlayerUnit.Position, p)
}

func (pf *PathFinder) OptimizeRoomsTraverseOrder() []data.Room {
	distanceMatrix := make(map[data.Room]map[data.Room]int)

	for _, room1 := range pf.data.Rooms {
		distanceMatrix[room1] = make(map[data.Room]int)
		for _, room2 := range pf.data.Rooms {
			if room1 != room2 {
				distance := DistanceFromPoint(room1.GetCenter(), room2.GetCenter())
				distanceMatrix[room1][room2] = distance
			} else {
				distanceMatrix[room1][room2] = 0
			}
		}
	}

	currentRoom := data.Room{}
	for _, r := range pf.data.Rooms {
		if r.IsInside(pf.data.PlayerUnit.Position) {
			currentRoom = r
		}
	}

	visited := make(map[data.Room]bool)
	order := []data.Room{currentRoom}
	visited[currentRoom] = true

	for len(order) < len(pf.data.Rooms) {
		nextRoom := data.Room{}
		minDistance := math.MaxInt

		// Find the nearest unvisited room
		for _, room := range pf.data.Rooms {
			if !visited[room] && distanceMatrix[currentRoom][room] < minDistance {
				nextRoom = room
				minDistance = distanceMatrix[currentRoom][room]
			}
		}

		// Add the next room to the order of visit
		order = append(order, nextRoom)
		visited[nextRoom] = true
		currentRoom = nextRoom
	}

	return order
}

func (pf *PathFinder) MoveThroughPath(p Path, walkDuration time.Duration) {
	if pf.data.CanTeleport() {
		pf.moveThroughPathTeleport(p)
	} else {
		pf.moveThroughPathWalk(p, walkDuration)
	}
}

func (pf *PathFinder) moveThroughPathWalk(p Path, walkDuration time.Duration) {
	// Calculate the max distance we can walk in the given duration
	maxDistance := int(float64(25) * walkDuration.Seconds())

	// Let's try to calculate how close to the window border we can go
	screenCords := data.Position{}
	for distance, pos := range p {
		screenX, screenY := pf.gameCoordsToScreenCords(p.From().X, p.From().Y, pos.X, pos.Y)

		// We reached max distance, let's stop (if we are not teleporting)
		if !pf.data.CanTeleport() && maxDistance > 0 && distance > maxDistance {
			break
		}

		// Prevent mouse overlap the HUD
		if screenY > int(float32(pf.gr.GameAreaSizeY)/1.19) {
			break
		}

		// We are getting out of the window, let's stop
		if screenX < 0 || screenY < 0 || screenX > pf.gr.GameAreaSizeX || screenY > pf.gr.GameAreaSizeY {
			break
		}
		screenCords = data.Position{X: screenX, Y: screenY}
	}

	pf.MoveCharacter(screenCords.X, screenCords.Y)
}

func (pf *PathFinder) moveThroughPathTeleport(p Path) {
	hudBoundary := int(float32(pf.gr.GameAreaSizeY) / 1.19)
	fromX, fromY := p.From().X, p.From().Y

	for i := len(p) - 1; i >= 0; i-- {
		pos := p[i]
		screenX, screenY := pf.gameCoordsToScreenCords(fromX, fromY, pos.X, pos.Y)

		if screenY > hudBoundary {
			continue
		}

		if screenX >= 0 && screenY >= 0 && screenX <= pf.gr.GameAreaSizeX && screenY <= pf.gr.GameAreaSizeY {
			worldPos := data.Position{
				X: pos.X + pf.data.AreaOrigin.X,
				Y: pos.Y + pf.data.AreaOrigin.Y,
			}

			usePacket := pf.cfg.PacketCasting.UseForTeleport && pf.packetSender != nil

			if usePacket {
				if pf.isMouseClickTeleportZone() {
					slog.Debug("Mouse click teleport zone detected, using mouse click instead of packet",
						slog.String("area", pf.data.PlayerUnit.Area.Area().Name),
					)
					usePacket = false
				} else {
					nearBoundary := pf.isNearAreaBoundary(worldPos, 60)
					if nearBoundary {
						slog.Debug("Near area boundary detected, using mouse click instead of packet",
							slog.Int("x", worldPos.X),
							slog.Int("y", worldPos.Y),
						)
						usePacket = false
					}
				}
			}

			if usePacket {
				pf.MoveCharacter(screenX, screenY, worldPos)
			} else {
				pf.MoveCharacter(screenX, screenY)
			}
			return
		}
	}
}

func (pf *PathFinder) GetLastPathIndexOnScreen(p Path) int {
	hudBoundary := int(float32(pf.gr.GameAreaSizeY) / 1.19)
	fromX, fromY := p.From().X, p.From().Y

	for i := len(p) - 1; i >= 0; i-- {
		pos := p[i]
		screenX, screenY := pf.gameCoordsToScreenCords(fromX, fromY, pos.X, pos.Y)

		// Prevent mouse overlap the HUD
		if screenY > hudBoundary {
			continue
		}

		// Check if coordinates are within screen bounds
		if screenX >= 0 && screenY >= 0 && screenX <= pf.gr.GameAreaSizeX && screenY <= pf.gr.GameAreaSizeY {
			return i
		}
	}

	return 0
}

func (pf *PathFinder) isNearAreaBoundary(pos data.Position, threshold int) bool {
	if pf.data.AreaData.Grid == nil {
		return false
	}

	distToLeft := pos.X - pf.data.AreaData.OffsetX
	distToRight := (pf.data.AreaData.OffsetX + pf.data.AreaData.Width) - pos.X
	distToTop := pos.Y - pf.data.AreaData.OffsetY
	distToBottom := (pf.data.AreaData.OffsetY + pf.data.AreaData.Height) - pos.Y

	minDistance := distToLeft
	if distToRight < minDistance {
		minDistance = distToRight
	}
	if distToTop < minDistance {
		minDistance = distToTop
	}
	if distToBottom < minDistance {
		minDistance = distToBottom
	}

	return minDistance <= threshold
}

func (pf *PathFinder) isMouseClickTeleportZone() bool {
	currentArea := pf.data.PlayerUnit.Area
	switch currentArea {
	case area.FlayerJungle, area.LowerKurast, area.RiverOfFlame:
		return true
	}
	return false
}

func (pf *PathFinder) MoveCharacter(x, y int, gamePos ...data.Position) {
	if pf.data.CanTeleport() {
		if pf.cfg.PacketCasting.UseForTeleport && pf.packetSender != nil && len(gamePos) > 0 {
			// Ensure Teleport skill is selected on right-click if using packet skill selection
			if pf.cfg.PacketCasting.UseForSkillSelection && pf.packetSender != nil {
				if pf.data.PlayerUnit.RightSkill != skill.Teleport {
					if err := pf.packetSender.SelectRightSkill(skill.Teleport); err == nil {
						utils.Sleep(50)
					}
				}
			}

			err := pf.packetSender.Teleport(gamePos[0])
			if err != nil {
				pf.hid.Click(game.RightButton, x, y)
			} else {
				utils.Sleep(int(pf.data.PlayerCastDuration().Milliseconds()))
			}
		} else {
			pf.hid.Click(game.RightButton, x, y)
		}
	} else {
		pf.hid.MovePointer(x, y)
		// If in town, use Left Click to move (fallback for users without Force Move bound)
		// In combat areas, we must use Force Move to avoid attacking monsters by accident
		if pf.data.PlayerUnit.Area.IsTown() {
			pf.hid.Click(game.LeftButton, x, y)
		} else {
			pf.pressForceMove(x, y)
		}
		utils.Sleep(50)
	}
}

// pressForceMove handles ForceMove input, detecting if it's bound to a mouse button
// and using Click instead of PressKeyBinding if so (since PressKeyBinding can't send
// mouse messages properly without coordinates)
func (pf *PathFinder) pressForceMove(x, y int) {
	forceMoveKey := pf.data.KeyBindings.ForceMove.Key1[0]
	if forceMoveKey == 0 || forceMoveKey == 255 {
		forceMoveKey = pf.data.KeyBindings.ForceMove.Key2[0]
	}
	// VK_LBUTTON(1), VK_RBUTTON(2), VK_MBUTTON(4), VK_XBUTTON1(5), VK_XBUTTON2(6) - note: 3 is VK_CANCEL
	if forceMoveKey >= 1 && forceMoveKey <= 6 && forceMoveKey != 3 {
		// ForceMove is bound to a mouse button - use left click at the cursor position
		pf.hid.Click(game.LeftButton, x, y)
	} else {
		// ForceMove is bound to a keyboard key - use PressKeyBinding
		pf.hid.PressKeyBinding(pf.data.KeyBindings.ForceMove)
	}
}

func (pf *PathFinder) GameCoordsToScreenCords(destinationX, destinationY int) (int, int) {
	return pf.gameCoordsToScreenCords(pf.data.PlayerUnit.Position.X, pf.data.PlayerUnit.Position.Y, destinationX, destinationY)
}

func (pf *PathFinder) gameCoordsToScreenCords(playerX, playerY, destinationX, destinationY int) (int, int) {
	// Calculate diff between current player position and destination
	diffX := destinationX - playerX
	diffY := destinationY - playerY

	// Transform cartesian movement (World) to isometric (screen)
	// Helpful documentation: https://clintbellanger.net/articles/isometric_math/
	screenX := int((float32(diffX-diffY) * 19.8) + float32(pf.gr.GameAreaSizeX/2))
	screenY := int((float32(diffX+diffY) * 9.9) + float32(pf.gr.GameAreaSizeY/2))

	return screenX, screenY
}

func IsNarrowMap(a area.ID) bool {
	switch a {
	case area.MaggotLairLevel1, area.MaggotLairLevel2, area.MaggotLairLevel3, area.ArcaneSanctuary, area.ClawViperTempleLevel2, area.RiverOfFlame, area.ChaosSanctuary:
		return true
	}

	return false
}

func DistanceFromPoint(from data.Position, to data.Position) int {
	first := math.Pow(float64(to.X-from.X), 2)
	second := math.Pow(float64(to.Y-from.Y), 2)

	return int(math.Sqrt(first + second))
}

func (pf *PathFinder) LineOfSight(origin data.Position, destination data.Position) bool {
	dx := int(math.Abs(float64(destination.X - origin.X)))
	dy := int(math.Abs(float64(destination.Y - origin.Y)))
	sx, sy := 1, 1

	if origin.X > destination.X {
		sx = -1
	}
	if origin.Y > destination.Y {
		sy = -1
	}

	err := dx - dy

	x, y := origin.X, origin.Y

	for {
		if !pf.data.AreaData.Grid.IsWalkable(data.Position{X: x, Y: y}) {
			return false
		}
		if x == destination.X && y == destination.Y {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x += sx
		}
		if e2 < dx {
			err += dx
			y += sy
		}
	}

	return true
}

func (pf *PathFinder) HasDoorBetween(origin data.Position, destination data.Position) (bool, *data.Object) {
	path, _, pathFound := pf.GetPathFrom(origin, destination)
	if !pathFound {
		if door, found := pf.GetClosestDoor(origin); found {
			return true, door
		}
		return false, nil
	}

	for _, o := range pf.data.Objects {
		if o.IsDoor() && o.Selectable && path.Intersects(*pf.data, o.Position, 4) {
			return true, &o
		}
	}

	return false, nil
}

// BeyondPosition calculates a new position that is a specified distance beyond the target position when viewed from the start position
func (pf *PathFinder) BeyondPosition(start, target data.Position, distance int) data.Position {
	// Calculate direction vector
	dx := float64(target.X - start.X)
	dy := float64(target.Y - start.Y)

	// Normalize
	length := math.Sqrt(dx*dx + dy*dy)
	if length == 0 {
		// If positions are identical, pick arbitrary direction
		dx = 1
		dy = 0
	} else {
		dx = dx / length
		dy = dy / length
	}

	// Return position extended beyond target
	return data.Position{
		X: target.X + int(dx*float64(distance)),
		Y: target.Y + int(dy*float64(distance)),
	}
}

func (pf *PathFinder) GetClosestDestructible(position data.Position) (*data.Object, bool) {
	breakableObjects := []object.Name{
		object.Barrel, object.Urn2, object.Urn3, object.Casket,
		object.Casket5, object.Casket6, object.LargeUrn1, object.LargeUrn4,
		object.LargeUrn5, object.Crate, object.HollowLog, object.Sarcophagus,
	}

	const immediateVicinity = 2.0
	var closestObject *data.Object
	minDistance := immediateVicinity

	// check for breakable objects
	for _, o := range pf.data.Objects {
		for _, breakableName := range breakableObjects {
			if o.Name == breakableName && o.Selectable {
				distanceToObj := utils.CalculateDistance(position, o.Position)
				if distanceToObj < minDistance {
					minDistance = distanceToObj
					closestObject = &o
				}
			}
		}
	}

	if closestObject != nil {
		return closestObject, true
	}

	return nil, false
}

func (pf *PathFinder) GetClosestDoor(position data.Position) (*data.Object, bool) {
	const immediateVicinity = 5.0
	var closestObject *data.Object
	minDistance := immediateVicinity

	// Then, check for doors. If a closer door is found, prioritize it.
	for _, o := range pf.data.Objects {
		if o.IsDoor() && o.Selectable {
			distanceToDoor := utils.CalculateDistance(position, o.Position)
			if distanceToDoor < immediateVicinity && distanceToDoor < minDistance {
				minDistance = distanceToDoor
				closestObject = &o
			}
		}
	}

	if closestObject != nil {
		return closestObject, true
	}

	return nil, false
}

func (pf *PathFinder) GetClosestChest(position data.Position, losCheck bool) (*data.Object, bool) {
	var closestObject *data.Object
	minDistance := 20.0

	// check for breakable objects
	for _, o := range pf.data.Objects {
		if o.Selectable {
			if !o.IsChest() && !o.IsSuperChest() {
				continue
			}

			distanceToObj := utils.CalculateDistance(position, o.Position)
			if distanceToObj < minDistance {
				if !losCheck || pf.LineOfSight(position, o.Position) {
					minDistance = distanceToObj
					closestObject = &o
				}
			}
		}
	}

	if closestObject != nil {
		return closestObject, true
	}

	return nil, false
}
