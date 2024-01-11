package bgammon

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"time"
)

var boardTopBlack = []byte("+13-14-15-16-17-18-+---+19-20-21-22-23-24-+")
var boardBottomBlack = []byte("+12-11-10--9--8--7-+---+-6--5--4--3--2--1-+")

var boardTopWhite = []byte("+24-23-22-21-20-19-+---+18-17-16-15-14-13-+")
var boardBottomWhite = []byte("+-1--2--3--4--5--6-+---+-7--8--9-10-11-12-+")

const (
	VariantBackgammon int8 = 0
	VariantAceyDeucey int8 = 1
	VariantTabula     int8 = 2
)

type Game struct {
	Started time.Time
	Ended   time.Time

	Player1 Player
	Player2 Player

	Variant int8 // 0 - Backgammon, 1 - Acey-deucey, 2 - Tabula.
	Board   []int8
	Turn    int8

	Roll1 int8
	Roll2 int8
	Roll3 int8 // Used in tabula games.

	Moves  [][]int8 // Pending moves.
	Winner int8

	Points        int8 // Points required to win the match.
	DoubleValue   int8 // Doubling cube value.
	DoublePlayer  int8 // Player that currently posesses the doubling cube.
	DoubleOffered bool // Whether the current player is offering a double.

	Reroll bool // Used in acey-deucey.

	boardStates   [][]int8  // One board state for each move to allow undoing a move.
	enteredStates [][2]bool // Player 1 entered state and Player 2 entered state for each move.

	// Fields after this point are provided for backwards-compatibility only and will eventually be removed.
	Acey bool // For Boxcars v1.2.1 and earlier.
}

func NewGame(variant int8) *Game {
	g := &Game{
		Variant:     variant,
		Board:       NewBoard(variant),
		Player1:     NewPlayer(1),
		Player2:     NewPlayer(2),
		Points:      1,
		DoubleValue: 1,
	}
	if variant == VariantBackgammon {
		g.Player1.Entered = true
		g.Player2.Entered = true
	} else {
		// Set backwards-compatible field.
		g.Acey = true
	}
	return g
}

func (g *Game) Copy() *Game {
	newGame := &Game{
		Started: g.Started,
		Ended:   g.Ended,

		Player1: g.Player1,
		Player2: g.Player2,

		Variant: g.Variant,
		Board:   make([]int8, len(g.Board)),
		Turn:    g.Turn,
		Roll1:   g.Roll1,
		Roll2:   g.Roll2,
		Roll3:   g.Roll3,
		Moves:   make([][]int8, len(g.Moves)),
		Winner:  g.Winner,

		Points:        g.Points,
		DoubleValue:   g.DoubleValue,
		DoublePlayer:  g.DoublePlayer,
		DoubleOffered: g.DoubleOffered,

		Reroll: g.Reroll,

		boardStates:   make([][]int8, len(g.boardStates)),
		enteredStates: make([][2]bool, len(g.enteredStates)),
	}
	copy(newGame.Board, g.Board)
	copy(newGame.Moves, g.Moves)
	copy(newGame.boardStates, g.boardStates)
	copy(newGame.enteredStates, g.enteredStates)
	return newGame
}

func (g *Game) NextTurn(reroll bool) {
	if g.Winner != 0 {
		return
	}

	if !reroll {
		var nextTurn int8 = 1
		if g.Turn == 1 {
			nextTurn = 2
		}
		g.Turn = nextTurn
	}

	g.Roll1, g.Roll2, g.Roll3 = 0, 0, 0
	g.Moves = g.Moves[:0]
	g.boardStates = g.boardStates[:0]
	g.enteredStates = g.enteredStates[:0]
}

func (g *Game) Reset() {
	if g.Variant != VariantBackgammon {
		g.Player1.Entered = false
		g.Player2.Entered = false
	}
	g.Board = NewBoard(g.Variant)
	g.Turn = 0
	g.Roll1 = 0
	g.Roll2 = 0
	g.Roll3 = 0
	g.Moves = nil
	g.DoubleValue = 1
	g.DoublePlayer = 0
	g.DoubleOffered = false
	g.Reroll = false
	g.boardStates = nil
	g.enteredStates = nil
}

func (g *Game) turnPlayer() Player {
	switch g.Turn {
	case 2:
		return g.Player2
	default:
		return g.Player1
	}
}

func (g *Game) opponentPlayer() Player {
	switch g.Turn {
	case 2:
		return g.Player1
	default:
		return g.Player2
	}
}

func (g *Game) SecondHalf(player int8, local bool) bool {
	if g.Variant != VariantTabula {
		return false
	}

	b := g.Board
	switch player {
	case 1:
		if b[SpaceBarPlayer] != 0 {
			return false
		} else if !g.Player1.Entered && b[SpaceHomePlayer] != 0 {
			return false
		}
	case 2:
		if b[SpaceBarOpponent] != 0 {
			return false
		} else if !g.Player2.Entered && b[SpaceHomeOpponent] != 0 {
			return false
		}
	default:
		log.Panicf("unknown player: %d", player)
	}

	for space := 1; space < 13; space++ {
		v := b[space]
		if (player == 1 && v > 0) || (player == 2 && v < 0) {
			return false
		}
	}

	return true
}

func (g *Game) setEntered() {
	if g.Variant == VariantBackgammon {
		return
	}
	if !g.Player1.Entered && g.Board[SpaceHomePlayer] == 0 {
		g.Player1.Entered = true
	} else if !g.Player2.Entered && g.Board[SpaceHomeOpponent] == 0 {
		g.Player2.Entered = true
	}
}

func (g *Game) addMove(move []int8) bool {
	opponentCheckers := OpponentCheckers(g.Board[move[1]], g.Turn)
	if opponentCheckers > 1 {
		return false
	}

	var delta int8 = 1
	if g.Turn == 2 {
		delta = -1
	}

	boardState := make([]int8, len(g.Board))
	copy(boardState, g.Board)
	g.boardStates = append(g.boardStates, boardState)
	g.enteredStates = append(g.enteredStates, [2]bool{g.Player1.Entered, g.Player2.Entered})

	g.Board[move[0]] -= delta
	if opponentCheckers == 1 { // Hit checker.
		g.Board[move[1]] = delta

		// Move opponent checker to bar.
		barSpace := SpaceBarOpponent
		if g.Turn == 2 {
			barSpace = SpaceBarPlayer
		}
		g.Board[barSpace] += delta * -1
	} else {
		g.Board[move[1]] += delta
	}

	g.Moves = append(g.Moves, []int8{move[0], move[1]})
	g.setEntered()
	return true
}

// AddLocalMove adds a move without performing any validation. This is useful when
// adding a move locally while waiting for an EventBoard response from the server.
func (g *Game) AddLocalMove(move []int8) bool {
	return g.addMove(move)
}

func (g *Game) ExpandMove(move []int8, currentSpace int8, moves [][]int8, local bool) ([][]int8, bool) {
	l := g.LegalMoves(local)
	var hitMoves [][]int8
	for _, m := range l {
		if OpponentCheckers(g.Board[m[1]], g.Turn) == 1 {
			hitMoves = append(hitMoves, m)
		}
	}
	for i := 0; i < 2; i++ {
		var checkMoves [][]int8
		if i == 0 { // Try moves that will hit an opponent's checker first.
			checkMoves = hitMoves
		} else {
			checkMoves = l
		}
		for _, lm := range checkMoves {
			if lm[0] != currentSpace {
				continue
			}

			newMoves := make([][]int8, len(moves))
			copy(newMoves, moves)
			newMoves = append(newMoves, []int8{lm[0], lm[1]})

			if lm[1] == move[1] {
				return newMoves, true
			}

			currentSpace = lm[1]

			gc := g.Copy()
			gc.addMove(lm)
			m, ok := gc.ExpandMove(move, currentSpace, newMoves, local)
			if ok {
				return m, ok
			}
		}
	}
	return nil, false
}

// AddMoves adds moves to the game state.  Adding a backwards move will remove the equivalent existing move.
func (g *Game) AddMoves(moves [][]int8, local bool) (bool, [][]int8) {
	if g.Player1.Name == "" || g.Player2.Name == "" || g.Winner != 0 {
		return false, nil
	}

	var addMoves [][]int8
	var undoMoves [][]int8

	gameCopy := g.Copy()

	validateOffset := 0
VALIDATEMOVES:
	for _, move := range moves {
		l := gameCopy.LegalMoves(local)
		for _, lm := range l {
			if lm[0] == move[0] && lm[1] == move[1] {
				addMoves = append(addMoves, []int8{move[0], move[1]})
				continue VALIDATEMOVES
			}
		}

		if len(gameCopy.Moves) > 0 {
			i := len(gameCopy.Moves) - 1 - validateOffset
			if i < 0 {
				return false, nil
			}
			gameMove := gameCopy.Moves[i]
			if move[0] == gameMove[1] && move[1] == gameMove[0] {
				undoMoves = append(undoMoves, []int8{gameMove[1], gameMove[0]})
				validateOffset++
				continue VALIDATEMOVES
			}
		}

		expandedMoves, ok := g.ExpandMove(move, move[0], nil, local)
		if ok {
			for _, expanded := range expandedMoves {
				addMoves = append(addMoves, []int8{expanded[0], expanded[1]})
			}
			continue VALIDATEMOVES
		}

		return false, nil
	}

	if len(addMoves) != 0 && len(undoMoves) != 0 {
		return false, nil
	}

	var checkWin bool
ADDMOVES:
	for _, move := range addMoves {
		l := gameCopy.LegalMoves(local)
		for _, lm := range l {
			if lm[0] == move[0] && lm[1] == move[1] {
				if !gameCopy.addMove(move) {
					return false, nil
				}

				if move[1] == SpaceHomePlayer || move[1] == SpaceHomeOpponent {
					checkWin = true
				}
				continue ADDMOVES
			}
		}
	}
	for _, move := range undoMoves {
		if len(gameCopy.Moves) > 0 {
			i := len(gameCopy.Moves) - 1
			if i < 0 {
				return false, nil
			}
			gameMove := gameCopy.Moves[i]
			if move[0] == gameMove[1] && move[1] == gameMove[0] {
				copy(gameCopy.Board, gameCopy.boardStates[i])
				gameCopy.Player1.Entered = gameCopy.enteredStates[i][0]
				gameCopy.Player2.Entered = gameCopy.enteredStates[i][1]
				gameCopy.boardStates = gameCopy.boardStates[:i]
				gameCopy.enteredStates = gameCopy.enteredStates[:i]
				gameCopy.Moves = gameCopy.Moves[:i]
				continue
			}
		}
		return false, nil
	}

	g.Board = append(g.Board[:0], gameCopy.Board...)
	g.Moves = gameCopy.Moves
	g.Player1.Entered, g.Player2.Entered = gameCopy.Player1.Entered, gameCopy.Player2.Entered
	g.boardStates = gameCopy.boardStates
	g.enteredStates = gameCopy.enteredStates

	if checkWin {
		entered := g.Player1.Entered
		if !local && g.Turn == 2 {
			entered = g.Player2.Entered
		}

		var foundChecker bool
		if g.Variant != VariantBackgammon && !entered {
			foundChecker = true
		} else {
			for space := 1; space <= 24; space++ {
				if PlayerCheckers(g.Board[space], g.Turn) != 0 {
					foundChecker = true
					break
				}
			}
		}

		if !foundChecker {
			g.Winner = g.Turn
		}
	}

	if len(addMoves) > 0 {
		return true, addMoves
	} else {
		return true, undoMoves
	}
}

func (g *Game) DiceRolls() []int8 {
	rolls := []int8{
		g.Roll1,
		g.Roll2,
	}
	if g.Variant == VariantTabula {
		rolls = append(rolls, g.Roll3)
	} else if g.Roll1 == g.Roll2 {
		rolls = append(rolls, g.Roll1, g.Roll2)
	}

	useDiceRoll := func(from, to int8) bool {
		if to == SpaceHomePlayer || to == SpaceHomeOpponent {
			needRoll := from
			if to == SpaceHomeOpponent || g.Variant == VariantTabula {
				needRoll = 25 - from
			}
			for i, roll := range rolls {
				if roll == needRoll {
					rolls = append(rolls[:i], rolls[i+1:]...)
					return true
				}
			}
			for i, roll := range rolls {
				if roll > needRoll {
					rolls = append(rolls[:i], rolls[i+1:]...)
					return true
				}
			}
			return false
		}

		diff := SpaceDiff(from, to, g.Variant)
		for i, roll := range rolls {
			if roll == diff {
				rolls = append(rolls[:i], rolls[i+1:]...)
				return true
			}
		}
		return false
	}

	for _, move := range g.Moves {
		if !useDiceRoll(move[0], move[1]) {
			return nil
		}
	}

	return rolls
}

func (g *Game) HaveDiceRoll(from int8, to int8) int8 {
	if g.Variant == VariantTabula && to > 12 && to < 25 && ((g.Turn == 1 && !g.Player1.Entered) || (g.Turn == 2 && !g.Player2.Entered)) {
		return 0
	} else if (to == SpaceHomePlayer || to == SpaceHomeOpponent) && !g.MayBearOff(g.Turn, false) {
		return 0
	}
	diff := SpaceDiff(from, to, g.Variant)
	if diff == 0 {
		return 0
	}
	var c int8
	for _, roll := range g.DiceRolls() {
		if roll == diff {
			c++
		}
	}
	return c
}

func (g *Game) HaveBearOffDiceRoll(diff int8) int8 {
	if diff == 0 {
		return 0
	}
	var c int8
	for _, roll := range g.DiceRolls() {
		if roll == diff || (roll > diff && g.Variant == VariantBackgammon) {
			c++
		}
	}
	return c
}

// totalMoves tries all legal moves in a game and returns all of the possible combinations of moves that a player may make.
func (g *Game) TotalMoves(local bool) [][][]int8 {
	var maxMoves int
	var allMoves [][][]int8
	for _, move := range g.LegalMoves(local) {
		for _, newMoves := range g._totalMoves(g.Moves, move, local) {
			if len(newMoves) > maxMoves {
				maxMoves = len(newMoves)
			} else if len(newMoves) < maxMoves {
				continue
			}
			allMoves = append(allMoves, newMoves)
		}
	}
	var newMoves [][][]int8
	for _, moves := range allMoves {
		if len(moves) == maxMoves {
			newMoves = append(newMoves, moves)
		}
	}
	return newMoves
}

// totalMoves tries all legal moves in a game and returns all of the possible combinations of moves that a player may make.
func (g *Game) _totalMoves(moves [][]int8, move []int8, local bool) [][][]int8 {
	gc := g.Copy()
	if !gc.addMove(move) {
		log.Panicf("failed to add move %+v to game %+v", move, g)
	}

	var allMoves [][][]int8
	{
		newMoves := append([][]int8{}, moves...)
		newMoves = append(newMoves, move)
		allMoves = append(allMoves, newMoves)
		maxMoves := len(newMoves)
		for _, m := range gc.LegalMoves(local) {
			for _, newMoves := range gc._totalMoves(newMoves, m, local) {
				if len(newMoves) > maxMoves {
					maxMoves = len(newMoves)
				} else if len(newMoves) < maxMoves {
					continue
				}
				allMoves = append(allMoves, newMoves)
			}
		}
	}

	var newMoves [][][]int8
TOTALMOVES:
	for _, m1 := range allMoves {
		for _, m2 := range newMoves {
			if movesEqual(m1, m2) {
				continue TOTALMOVES
			}
		}
		newMoves = append(newMoves, m1)
	}
	return allMoves
}

func (g *Game) LegalMoves(local bool) [][]int8 {
	if g.Winner != 0 || g.Roll1 == 0 || g.Roll2 == 0 {
		return nil
	}

	var moves [][]int8
	var movesFound = make(map[int8]bool)

	var mustEnter bool
	var barSpace int8
	if PlayerCheckers(g.Board[SpaceBarPlayer], g.Turn) > 0 {
		mustEnter = true
		barSpace = SpaceBarPlayer
	} else if PlayerCheckers(g.Board[SpaceBarOpponent], g.Turn) > 0 {
		mustEnter = true
		barSpace = SpaceBarOpponent
	}
	if mustEnter { // Must enter from bar.
		from, to := HomeRange(g.opponentPlayer().Number, g.Variant)
		if g.Variant == VariantTabula {
			from, to = 1, 6
		}
		IterateSpaces(from, to, g.Variant, func(homeSpace int8, spaceCount int8) {
			if false && movesFound[barSpace*100+homeSpace] {
				return
			}
			available := g.HaveDiceRoll(barSpace, homeSpace)
			if available == 0 {
				return
			}
			opponentCheckers := OpponentCheckers(g.Board[homeSpace], g.Turn)
			if opponentCheckers <= 1 {
				moves = append(moves, []int8{barSpace, homeSpace})
				movesFound[barSpace*100+homeSpace] = true
			}
		})
	} else {
		mayBearOff := g.MayBearOff(g.Turn, false)
		for sp := range g.Board {
			space := int8(sp)
			if space == SpaceBarPlayer || space == SpaceBarOpponent { // Handled above.
				continue
			} else if space == SpaceHomePlayer || space == SpaceHomeOpponent {
				homeSpace := SpaceHomePlayer
				entered := g.Player1.Entered
				if g.Turn == 2 {
					homeSpace = SpaceHomeOpponent
					entered = g.Player2.Entered
				}
				if g.Variant == VariantBackgammon || space != homeSpace || entered {
					continue
				}
			}

			checkers := g.Board[space]
			playerCheckers := PlayerCheckers(checkers, g.Turn)
			if playerCheckers == 0 {
				continue
			}

			if mayBearOff {
				homeSpace := SpaceHomePlayer
				if g.Turn == 2 {
					homeSpace = SpaceHomeOpponent
				}
				if false && movesFound[space*100+homeSpace] {
					continue
				}
				available := g.HaveBearOffDiceRoll(SpaceDiff(space, homeSpace, g.Variant))
				if available > 0 {
					ok := true
					if g.Variant == VariantBackgammon && g.HaveDiceRoll(space, homeSpace) == 0 {
						_, homeEnd := HomeRange(g.Turn, g.Variant)
						if g.Turn == 2 && g.Variant != VariantTabula {
							for homeSpace := space - 1; homeSpace >= homeEnd; homeSpace-- {
								if PlayerCheckers(g.Board[homeSpace], g.Turn) != 0 {
									ok = false
									break
								}
							}
						} else {
							for homeSpace := space + 1; homeSpace <= homeEnd; homeSpace++ {
								if PlayerCheckers(g.Board[homeSpace], g.Turn) != 0 {
									ok = false
									break
								}
							}
						}
					}
					if ok {
						moves = append(moves, []int8{space, homeSpace})
						movesFound[space*100+homeSpace] = true
					}
				}
			}

			// Move normally.
			var lastSpace int8 = 0
			if g.Turn == 2 || g.Variant == VariantTabula {
				lastSpace = 25
			}

			f := func(to int8, spaceCount int8) {
				if false && movesFound[space*100+to] {
					return
				}
				available := g.HaveDiceRoll(space, to)
				if available == 0 {
					return
				}

				opponentCheckers := OpponentCheckers(g.Board[to], g.Turn)
				if opponentCheckers <= 1 {
					moves = append(moves, []int8{space, to})
					movesFound[space*100+to] = true
				}
			}
			if space == SpaceHomePlayer {
				iterateSpace := int8(25)
				if g.Variant == VariantTabula {
					iterateSpace = 1
				}
				IterateSpaces(iterateSpace, lastSpace, g.Variant, f)
			} else if space == SpaceHomeOpponent {
				IterateSpaces(1, lastSpace, g.Variant, f)
			} else {
				IterateSpaces(space, lastSpace, g.Variant, f)
			}
		}
	}

	// Simulate all possible moves to their final value and only allow moves that will achieve the maximum total moves.
	var maxMoves int8
	moveCounts := make([]int8, len(moves))
	for i, move := range moves {
		var moveCount int
		allMoves := g._totalMoves(g.Moves, move, local)
		if len(allMoves) > 0 {
			moveCount = len(allMoves[0])
		}
		moveCounts[i] = int8(moveCount)
		if moveCounts[i] > maxMoves {
			maxMoves = moveCounts[i]
		}
	}
	if maxMoves > 1 {
		var newMoves [][]int8
		for i, move := range moves {
			if moveCounts[i] >= maxMoves {
				newMoves = append(newMoves, move)
			}
		}
		moves = newMoves
	}

	replaceSpace := func(i int8) int8 {
		if g.Turn == 1 && i == SpaceHomeOpponent {
			return SpaceHomePlayer
		} else if g.Turn == 1 && i == SpaceBarOpponent {
			return SpaceBarPlayer
		} else if g.Turn == 2 && i == SpaceHomePlayer {
			return SpaceHomeOpponent
		} else if g.Turn == 2 && i == SpaceBarPlayer {
			return SpaceBarOpponent
		}
		return i
	}
	for i := range moves {
		for j := range moves[i] {
			moves[i][j] = replaceSpace(moves[i][j])
		}
	}

	return moves
}

// MayBearOff returns whether the provided player may bear checkers off of the board.
func (g *Game) MayBearOff(player int8, local bool) bool {
	if PlayerCheckers(g.Board[SpaceBarPlayer], player) > 0 || PlayerCheckers(g.Board[SpaceBarOpponent], player) > 0 {
		return false
	} else if (player == 1 && !g.Player1.Entered) || (player == 2 && !g.Player2.Entered) {
		return false
	} else if g.Variant == VariantTabula {
		return g.SecondHalf(player, local)
	}

	homeStart, homeEnd := int8(1), int8(6)
	if !local {
		homeStart, homeEnd = HomeRange(player, g.Variant)
		homeStart, homeEnd = minInt(homeStart, homeEnd), maxInt(homeStart, homeEnd)
	}
	for i := int8(1); i <= 24; i++ {
		if (i < homeStart || i > homeEnd) && PlayerCheckers(g.Board[i], player) > 0 {
			return false
		}
	}
	return true
}

func (g *Game) RenderSpace(player int8, space int8, spaceValue int8, legalMoves [][]int8) []byte {
	var playerColor = "x"
	var opponentColor = "o"
	if player == 2 {
		playerColor = "o"
		opponentColor = "x"
	}

	var pieceColor string
	value := g.Board[space]
	if space == SpaceBarPlayer {
		pieceColor = playerColor
	} else if space == SpaceBarOpponent {
		pieceColor = opponentColor
	} else {
		if value < 0 {
			pieceColor = "o"
		} else if value > 0 {
			pieceColor = "x"
		} else {
			pieceColor = playerColor
		}
	}

	abs := value
	if value < 0 {
		abs = value * -1
	}

	top := space > 12
	if player == 2 {
		top = !top
	}

	var firstDigit int8 = 4
	var secondDigit int8 = 5
	if !top {
		firstDigit = 5
		secondDigit = 4
	}

	var firstNumeral string
	var secondNumeral string
	if abs > 5 {
		if abs > 9 {
			firstNumeral = "1"
		} else {
			firstNumeral = strconv.Itoa(int(abs))
		}
		if abs > 9 {
			secondNumeral = strconv.Itoa(int(abs) - 10)
		}

		if spaceValue == firstDigit && (!top || abs > 9) {
			pieceColor = firstNumeral
		} else if spaceValue == secondDigit && abs > 9 {
			pieceColor = secondNumeral
		} else if top && spaceValue == secondDigit {
			pieceColor = firstNumeral
		}
	}

	if abs > 5 {
		abs = 5
	}

	var r []byte
	if abs > 0 && spaceValue <= abs {
		r = []byte(pieceColor)
	} else {
		r = []byte(" ")
	}
	return append(append([]byte(" "), r...), ' ')
}

func (g *Game) BoardState(player int8, local bool) []byte {
	var t bytes.Buffer

	playerRating := "0"
	opponentRating := "0"

	var white bool
	if player == 2 {
		white = true
	}

	var opponentName = g.Player2.Name
	var playerName = g.Player1.Name
	if playerName == "" {
		playerName = "Waiting..."
	}
	if opponentName == "" {
		opponentName = "Waiting..."
	}
	if white {
		playerName, opponentName = opponentName, playerName
	}

	var playerColor = "x"
	var opponentColor = "o"
	playerRoll := g.Roll1
	opponentRoll := g.Roll2
	if white {
		playerColor = "o"
		opponentColor = "x"
		playerRoll = g.Roll2
		opponentRoll = g.Roll1
	}

	if white {
		t.Write(boardTopWhite)
	} else {
		t.Write(boardTopBlack)
	}
	t.WriteString(" ")
	t.WriteByte('\n')

	legalMoves := g.LegalMoves(local)
	space := func(row int8, col int8) []byte {
		var spaceValue int8 = row + 1
		if row > 5 {
			spaceValue = 5 - (row - 6)
		}

		if col == -1 {
			if row <= 4 {
				return g.RenderSpace(player, SpaceBarOpponent, spaceValue, legalMoves)
			}
			return g.RenderSpace(player, SpaceBarPlayer, spaceValue, legalMoves)
		}

		var space int8
		if white {
			space = 24 - col
			if row > 5 {
				space = 1 + col
			}
		} else {
			space = 13 + col
			if row > 5 {
				space = 12 - col
			}
		}

		if row == 5 {
			return []byte("   ")
		}

		return g.RenderSpace(player, space, spaceValue, legalMoves)
	}

	const verticalBar rune = '│'
	for i := int8(0); i < 11; i++ {
		t.WriteRune(verticalBar)
		t.Write([]byte(""))
		for j := int8(0); j < 12; j++ {
			t.Write(space(i, j))

			if j == 5 {
				t.WriteRune(verticalBar)
				t.Write(space(i, -1))
				t.WriteRune(verticalBar)
			}
		}

		t.Write([]byte("" + string(verticalBar) + "  "))

		if i == 0 {
			t.Write([]byte(opponentColor + " " + opponentName + " (" + opponentRating + ")"))
			if g.Board[SpaceHomeOpponent] != 0 {
				v := g.Board[SpaceHomeOpponent]
				if v < 0 {
					v *= -1
				}
				t.Write([]byte(fmt.Sprintf("  %d off", v)))
			}
		} else if i == 2 {
			if g.Turn == 0 {
				if g.Player1.Name != "" && g.Player2.Name != "" {
					if opponentRoll != 0 {
						t.Write([]byte(fmt.Sprintf("  %d", opponentRoll)))
					} else {
						t.Write([]byte("  -"))
					}
				}
			} else if g.Turn != player {
				if g.Roll1 > 0 {
					t.Write([]byte(fmt.Sprintf("  %d  %d  ", g.Roll1, g.Roll2)))
					if g.Roll3 != 0 {
						t.Write([]byte(fmt.Sprintf("%d  ", g.Roll3)))
					}
				} else if opponentName != "" {
					t.Write([]byte("  -  -  "))
				}
			}
		} else if i == 8 {
			if g.Turn == 0 {
				if g.Player1.Name != "" && g.Player2.Name != "" {
					if playerRoll != 0 {
						t.Write([]byte(fmt.Sprintf("  %d", playerRoll)))
					} else {
						t.Write([]byte("  -"))
					}
				}
			} else if g.Turn == player {
				if g.Roll1 > 0 {
					t.Write([]byte(fmt.Sprintf("  %d  %d  ", g.Roll1, g.Roll2)))
					if g.Roll3 != 0 {
						t.Write([]byte(fmt.Sprintf("%d  ", g.Roll3)))
					}
				} else if playerName != "" {
					t.Write([]byte("  -  -  "))
				}
			}
		} else if i == 10 {
			t.Write([]byte(playerColor + " " + playerName + " (" + playerRating + ")"))
			if g.Board[SpaceHomePlayer] != 0 {
				v := g.Board[SpaceHomePlayer]
				if v < 0 {
					v *= -1
				}
				t.Write([]byte(fmt.Sprintf("  %d off", v)))
			}
		}

		t.Write([]byte(" "))
		t.WriteByte('\n')
	}

	if white {
		t.Write(boardBottomWhite)
	} else {
		t.Write(boardBottomBlack)
	}
	t.WriteString("                 \n")

	return t.Bytes()
}

func SpaceDiff(from int8, to int8, variant int8) int8 {
	switch {
	case from < 0 || from > 27 || to < 0 || to > 27:
		return 0
	case to == SpaceBarPlayer || to == SpaceBarOpponent:
		return 0
	case (from == SpaceBarPlayer || from == SpaceBarOpponent) && (to == SpaceBarPlayer || to == SpaceBarOpponent || to == SpaceHomePlayer || to == SpaceHomeOpponent):
		return 0
	case to == SpaceHomePlayer:
		if variant == VariantTabula {
			return 25 - from
		}
		return from
	case to == SpaceHomeOpponent:
		return 25 - from
	case from == SpaceHomePlayer || from == SpaceHomeOpponent:
		switch variant {
		case VariantAceyDeucey:
			if from == SpaceHomePlayer {
				return 25 - to
			} else {
				return to
			}
		case VariantTabula:
			return to
		}
		return 0
	case from == SpaceBarPlayer:
		if variant == VariantTabula {
			return to
		}
		return 25 - to
	case from == SpaceBarOpponent:
		return to
	default:
		diff := to - from
		if diff < 0 {
			return diff * -1
		}
		return diff
	}
}

func IterateSpaces(from int8, to int8, variant int8, f func(space int8, spaceCount int8)) {
	if from == to || from < 0 || from > 25 || to < 0 || to > 25 {
		return
	} else if variant == VariantBackgammon {
		if from == 0 {
			from = 1
		} else if from == 25 {
			from = 24
		}
	}
	var i int8 = 1
	if to > from {
		for space := from; space <= to; space++ {
			f(space, i)
			i++
		}
	} else {
		for space := from; space >= to; space-- {
			f(space, i)
			i++
		}
	}
}

func PlayerCheckers(checkers int8, player int8) int8 {
	if player == 1 {
		if checkers > 0 {
			return checkers
		}
		return 0
	} else {
		if checkers < 0 {
			return checkers * -1
		}
		return 0
	}
}

func OpponentCheckers(checkers int8, player int8) int8 {
	if player == 2 {
		if checkers > 0 {
			return checkers
		}
		return 0
	} else {
		if checkers < 0 {
			return checkers * -1
		}
		return 0
	}
}

func FlipSpace(space int8, player int8, variant int8) int8 {
	if player == 1 {
		return space
	}
	if space < 1 || space > 24 {
		switch space {
		case SpaceHomePlayer:
			return SpaceHomeOpponent
		case SpaceHomeOpponent:
			return SpaceHomePlayer
		case SpaceBarPlayer:
			return SpaceBarOpponent
		case SpaceBarOpponent:
			return SpaceBarPlayer
		default:
			return -1
		}
	}
	if variant == VariantTabula {
		return space
	}
	return 24 - space + 1
}

func FlipMoves(moves [][]int8, player int8, variant int8) [][]int8 {
	m := make([][]int8, len(moves))
	for i := range moves {
		m[i] = []int8{FlipSpace(moves[i][0], player, variant), FlipSpace(moves[i][1], player, variant)}
	}
	return m
}

func FormatSpace(space int8) []byte {
	if space >= 1 && space <= 24 {
		return []byte(strconv.Itoa(int(space)))
	} else if space == SpaceBarPlayer || space == SpaceBarOpponent {
		return []byte("bar")
	} else if space == SpaceHomePlayer || space == SpaceHomeOpponent {
		return []byte("off")
	}
	return []byte("?")
}

func FormatMoves(moves [][]int8) []byte {
	if len(moves) == 0 {
		return []byte("none")
	}

	var out bytes.Buffer
	for i := range moves {
		if i != 0 {
			out.WriteByte(' ')
		}
		out.Write([]byte(fmt.Sprintf("%s/%s", FormatSpace(moves[i][0]), FormatSpace(moves[i][1]))))
	}
	return out.Bytes()
}

func FormatAndFlipMoves(moves [][]int8, player int8, variant int8) []byte {
	return FormatMoves(FlipMoves(moves, player, variant))
}

func ValidSpace(space int8) bool {
	return space >= 0 && space <= 27
}

func movesEqual(a [][]int8, b [][]int8) bool {
	l := len(a)
	if len(b) != l {
		return false
	}
	switch l {
	case 0:
		return true
	case 1:
		return a[0][0] == b[0][0] && a[0][1] == b[0][1]
	case 2:
		return (a[0][0] == b[0][0] && a[0][1] == b[0][1] && a[1][0] == b[1][0] && a[1][1] == b[1][1]) || // 1, 2
			(a[0][0] == b[1][0] && a[0][1] == b[1][1] && a[1][0] == b[0][0] && a[1][1] == b[0][1]) // 2, 1
	case 3:
		if a[0][0] == b[0][0] && a[0][1] == b[0][1] { // 1
			if (a[1][0] == b[1][0] && a[1][1] == b[1][1] && a[2][0] == b[2][0] && a[2][1] == b[2][1]) || // 2, 3
				(a[1][0] == b[2][0] && a[1][1] == b[2][1] && a[2][0] == b[1][0] && a[2][1] == b[1][1]) { // 3, 2
				return true
			}
		}
		if a[0][0] == b[1][0] && a[0][1] == b[1][1] { // 2
			if (a[1][0] == b[0][0] && a[1][1] == b[0][1] && a[2][0] == b[2][0] && a[2][1] == b[2][1]) ||
				(a[1][0] == b[2][0] && a[1][1] == b[2][1] && a[2][0] == b[0][0] && a[2][1] == b[0][1]) {
				return true
			}
		}
		if a[0][0] == b[2][0] && a[0][1] == b[2][1] { // 3
			if (a[1][0] == b[0][0] && a[1][1] == b[0][1] && a[2][0] == b[1][0] && a[2][1] == b[1][1]) || // 1, 2
				(a[1][0] == b[1][0] && a[1][1] == b[1][1] && a[2][0] == b[0][0] && a[2][1] == b[0][1]) { // 2, 1
				return true
			}
		}
		return false
	case 4:
		if a[0][0] == b[0][0] && a[0][1] == b[0][1] { // 1
			if a[1][0] == b[1][0] && a[1][1] == b[1][1] { // 2
				if (a[2][0] == b[2][0] && a[2][1] == b[2][1] && a[3][0] == b[3][0] && a[3][1] == b[3][1]) || // 3,4
					(a[2][0] == b[3][0] && a[2][1] == b[3][1] && a[3][0] == b[2][0] && a[3][1] == b[2][1]) { // 4,3
					return true
				}
			}
			if a[1][0] == b[2][0] && a[1][1] == b[2][1] { // 3
				if (a[2][0] == b[1][0] && a[2][1] == b[1][1] && a[3][0] == b[3][0] && a[3][1] == b[3][1]) || // 2,4
					(a[2][0] == b[3][0] && a[2][1] == b[3][1] && a[3][0] == b[1][0] && a[3][1] == b[1][1]) { // 4,2
					return true
				}
			}
			if a[1][0] == b[3][0] && a[1][1] == b[3][1] { // 4
				if (a[2][0] == b[2][0] && a[2][1] == b[2][1] && a[3][0] == b[1][0] && a[3][1] == b[1][1]) || // 3,2
					(a[2][0] == b[1][0] && a[2][1] == b[1][1] && a[3][0] == b[2][0] && a[3][1] == b[2][1]) { // 2,3
					return true
				}
			}
		}
		if a[0][0] == b[1][0] && a[0][1] == b[1][1] { // 2
			if a[1][0] == b[0][0] && a[1][1] == b[0][1] { // 1
				if (a[2][0] == b[2][0] && a[2][1] == b[2][1] && a[3][0] == b[3][0] && a[3][1] == b[3][1]) || // 3,4
					(a[2][0] == b[3][0] && a[2][1] == b[3][1] && a[3][0] == b[2][0] && a[3][1] == b[2][1]) { // 4,3
					return true
				}
			}
			if a[1][0] == b[2][0] && a[1][1] == b[2][1] { // 3
				if (a[2][0] == b[3][0] && a[2][1] == b[3][1] && a[3][0] == b[0][0] && a[3][1] == b[0][1]) || // 4,1
					(a[2][0] == b[0][0] && a[2][1] == b[0][1] && a[3][0] == b[3][0] && a[3][1] == b[3][1]) { // 1,4
					return true
				}
			}
			if a[1][0] == b[3][0] && a[1][1] == b[3][1] { // 4
				if (a[2][0] == b[2][0] && a[2][1] == b[2][1] && a[3][0] == b[0][0] && a[3][1] == b[0][1]) || // 3,1
					(a[2][0] == b[0][0] && a[2][1] == b[0][1] && a[3][0] == b[2][0] && a[3][1] == b[2][1]) { // 1,3
					return true
				}
			}
		}
		if a[0][0] == b[2][0] && a[0][1] == b[2][1] { // 3
			if a[1][0] == b[0][0] && a[1][1] == b[0][1] { // 1
				if (a[2][0] == b[1][0] && a[2][1] == b[1][1] && a[3][0] == b[3][0] && a[3][1] == b[3][1]) || // 2,4
					(a[2][0] == b[3][0] && a[2][1] == b[3][1] && a[3][0] == b[1][0] && a[3][1] == b[1][1]) { // 4,2
					return true
				}
			}
			if a[1][0] == b[1][0] && a[1][1] == b[1][1] { // 2
				if (a[2][0] == b[0][0] && a[2][1] == b[0][1] && a[3][0] == b[3][0] && a[3][1] == b[3][1]) || // 1,4
					(a[2][0] == b[3][0] && a[2][1] == b[3][1] && a[3][0] == b[0][0] && a[3][1] == b[0][1]) { // 4,1
					return true
				}
			}
			if a[1][0] == b[3][0] && a[1][1] == b[3][1] { // 4
				if (a[2][0] == b[1][0] && a[2][1] == b[1][1] && a[3][0] == b[0][0] && a[3][1] == b[0][1]) || // 2,1
					(a[2][0] == b[0][0] && a[2][1] == b[0][1] && a[3][0] == b[1][0] && a[3][1] == b[1][1]) { // 1,2
					return true
				}
			}
		}
		if a[0][0] == b[3][0] && a[0][1] == b[3][1] { // 4
			if a[1][0] == b[0][0] && a[1][1] == b[0][1] { // 1
				if (a[2][0] == b[2][0] && a[2][1] == b[2][1] && a[3][0] == b[1][0] && a[3][1] == b[1][1]) || // 3,2
					(a[2][0] == b[1][0] && a[2][1] == b[1][1] && a[3][0] == b[2][0] && a[3][1] == b[2][1]) { // 2,3
					return true
				}
			}
			if a[1][0] == b[1][0] && a[1][1] == b[1][1] { // 2
				if (a[2][0] == b[0][0] && a[2][1] == b[0][1] && a[3][0] == b[2][0] && a[3][1] == b[2][1]) || // 1,3
					(a[2][0] == b[2][0] && a[2][1] == b[2][1] && a[3][0] == b[0][0] && a[3][1] == b[0][1]) { // 3,1
					return true
				}
			}
			if a[1][0] == b[2][0] && a[1][1] == b[2][1] { // 3
				if (a[2][0] == b[0][0] && a[2][1] == b[0][1] && a[3][0] == b[1][0] && a[3][1] == b[1][1]) || // 1,2
					(a[2][0] == b[1][0] && a[2][1] == b[1][1] && a[3][0] == b[0][0] && a[3][1] == b[0][1]) { // 2,1
					return true
				}
			}
		}
		return false
	default:
		log.Panicf("more than 4 moves were provided: %+v %+v", a, b)
		return false
	}
}
