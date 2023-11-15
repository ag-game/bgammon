package main

import (
	"bufio"
	"bytes"
	"time"

	"code.rocket9labs.com/tslocum/bgammon"
)

type serverGame struct {
	id         int
	created    int64
	lastActive int64
	name       []byte
	password   []byte
	client1    *serverClient
	client2    *serverClient
	allowed1   []byte
	allowed2   []byte
	rematch    int
	rejoin1    bool
	rejoin2    bool
	*bgammon.Game
}

func newServerGame(id int) *serverGame {
	now := time.Now().Unix()
	return &serverGame{
		id:         id,
		created:    now,
		lastActive: now,
		Game:       bgammon.NewGame(),
	}
}

func (g *serverGame) roll(player int) bool {
	if g.client1 == nil || g.client2 == nil || g.Winner != 0 {
		return false
	}

	if g.Turn == 0 {
		if player == 1 {
			if g.Roll1 != 0 {
				return false
			}
			g.Roll1 = randInt(6) + 1
		} else {
			if g.Roll2 != 0 {
				return false
			}
			g.Roll2 = randInt(6) + 1
		}

		if g.Started.IsZero() {
			g.Started = time.Now()
		}

		// Only allow the same players to rejoin the game.
		if g.allowed1 == nil {
			g.allowed1, g.allowed2 = g.client1.name, g.client2.name
		}
		return true
	} else if player != g.Turn || g.Roll1 != 0 || g.Roll2 != 0 {
		return false
	}

	g.Roll1 = randInt(6) + 1
	g.Roll2 = randInt(6) + 1
	return true
}

func (g *serverGame) sendBoard(client *serverClient) {
	if client.json {
		ev := &bgammon.EventBoard{
			GameState: bgammon.GameState{
				Game:         g.Game,
				PlayerNumber: client.playerNumber,
				Available:    g.LegalMoves(false),
			},
		}

		// Reverse spaces for white.
		if client.playerNumber == 2 {
			ev.GameState.Game = ev.GameState.Copy()

			ev.GameState.PlayerNumber = 1
			ev.GameState.Player1, ev.GameState.Player2 = ev.GameState.Player2, ev.GameState.Player1
			ev.GameState.Player1.Number = 1
			ev.GameState.Player2.Number = 2

			switch ev.GameState.Turn {
			case 1:
				ev.GameState.Turn = 2
			case 2:
				ev.GameState.Turn = 1
			}

			switch ev.GameState.DoublePlayer {
			case 1:
				ev.GameState.DoublePlayer = 2
			case 2:
				ev.GameState.DoublePlayer = 1
			}

			switch ev.GameState.Winner {
			case 1:
				ev.GameState.Winner = 2
			case 2:
				ev.GameState.Winner = 1
			}

			if ev.GameState.Roll1 == 0 || ev.GameState.Roll2 == 0 {
				ev.GameState.Roll1, ev.GameState.Roll2 = ev.GameState.Roll2, ev.GameState.Roll1
			}

			// Flip board.
			for space := 1; space <= 24; space++ {
				ev.Board[space] = g.Game.Board[bgammon.FlipSpace(space, client.playerNumber)] * -1
			}
			ev.Board[bgammon.SpaceHomePlayer], ev.Board[bgammon.SpaceHomeOpponent] = ev.Board[bgammon.SpaceHomeOpponent]*-1, ev.Board[bgammon.SpaceHomePlayer]*-1
			ev.Board[bgammon.SpaceBarPlayer], ev.Board[bgammon.SpaceBarOpponent] = ev.Board[bgammon.SpaceBarOpponent]*-1, ev.Board[bgammon.SpaceBarPlayer]*-1

			ev.Moves = bgammon.FlipMoves(g.Game.Moves, client.playerNumber)

			legalMoves := g.LegalMoves(false)
			for i := range ev.GameState.Available {
				ev.GameState.Available[i][0], ev.GameState.Available[i][1] = bgammon.FlipSpace(legalMoves[i][0], client.playerNumber), bgammon.FlipSpace(legalMoves[i][1], client.playerNumber)
			}
		}

		// Sort available moves.
		bgammon.SortMoves(ev.Available)

		client.sendEvent(ev)
		return
	}

	scanner := bufio.NewScanner(bytes.NewReader(g.BoardState(client.playerNumber, false)))
	for scanner.Scan() {
		client.sendNotice(string(scanner.Bytes()))
	}
}

func (g *serverGame) playerCount() int {
	c := 0
	if g.client1 != nil {
		c++
	}
	if g.client2 != nil {
		c++
	}
	return c
}

func (g *serverGame) eachClient(f func(client *serverClient)) {
	if g.client1 != nil {
		f(g.client1)
	}
	if g.client2 != nil {
		f(g.client2)
	}
}

func (g *serverGame) addClient(client *serverClient) (bool, string) {
	if g.allowed1 != nil && !bytes.Equal(client.name, g.allowed1) && !bytes.Equal(client.name, g.allowed2) {
		return false, "Match has already started."
	}

	var playerNumber int
	defer func() {
		if playerNumber == 0 {
			return
		}

		ev := &bgammon.EventJoined{
			GameID:       g.id,
			PlayerNumber: playerNumber,
		}
		ev.Player = string(client.name)

		client.sendEvent(ev)
		g.sendBoard(client)

		opponent := g.opponent(client)
		if opponent != nil {
			opponent.sendEvent(ev)
			g.sendBoard(opponent)
		}

		if playerNumber == 1 {
			g.rejoin1 = true
		} else {
			g.rejoin2 = true
		}
	}()
	switch {
	case g.client1 != nil && g.client2 != nil:
		// Do not assign player number.
	case g.client1 != nil:
		g.client2 = client
		g.Player2.Name = string(client.name)
		client.playerNumber = 2
		playerNumber = 2
	case g.client2 != nil:
		g.client1 = client
		g.Player1.Name = string(client.name)
		client.playerNumber = 1
		playerNumber = 1
	default:
		if randInt(2) == 0 {
			g.client1 = client
			g.Player1.Name = string(client.name)
			client.playerNumber = 1
			playerNumber = 1
		} else {
			g.client2 = client
			g.Player2.Name = string(client.name)
			client.playerNumber = 2
			playerNumber = 2
		}
	}

	ok := playerNumber != 0
	var reason string
	if !ok {
		reason = "Match is full."
	}
	return ok, reason
}

func (g *serverGame) removeClient(client *serverClient) {
	var playerNumber int
	defer func() {
		if playerNumber == 0 {
			return
		}

		ev := &bgammon.EventLeft{}
		ev.Player = string(client.name)

		client.sendEvent(ev)
		if !client.json {
			g.sendBoard(client)
		}

		var opponent *serverClient
		if playerNumber == 1 && g.client2 != nil {
			opponent = g.client2
		} else if playerNumber == 2 && g.client1 != nil {
			opponent = g.client1
		}
		if opponent != nil {
			opponent.sendEvent(ev)
			if !opponent.json {
				g.sendBoard(opponent)
			}
		}

		client.playerNumber = 0
	}()
	switch {
	case g.client1 == client:
		g.client1 = nil
		g.Player1.Name = ""
		playerNumber = 1
	case g.client2 == client:
		g.client2 = nil
		g.Player2.Name = ""
		playerNumber = 2
	default:
		return
	}
}

func (g *serverGame) opponent(client *serverClient) *serverClient {
	if g.client1 == client {
		return g.client2
	} else if g.client2 == client {
		return g.client1
	}
	return nil
}

func (g *serverGame) terminated() bool {
	return g.client1 == nil && g.client2 == nil
}
