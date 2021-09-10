package main

import (
	"embed"
	"fmt"
	"image/color"
	_ "image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	logging "github.com/tsujio/game-logging-server/client"
	"github.com/tsujio/game-util/dotutil"
	"github.com/tsujio/game-util/resourceutil"
	"github.com/tsujio/game-util/touchutil"
)

const (
	gameName          = "wave-interference"
	screenWidth       = 640
	screenHeight      = 480
	screenCenterX     = screenWidth / 2
	screenCenterY     = screenHeight / 2
	surfaceBaseHeight = screenHeight / 3 * 2
	waveSpeed         = 2
	coinSpeed         = 1
	sharkSpeed        = 0.5
	humanR            = 10
	coinR             = 15
	sharkR            = 10
)

//go:embed resources/*.ttf resources/*.dat resources/bgm-*.wav
var resources embed.FS

var (
	fontL, fontM, fontS = resourceutil.ForceLoadFont(resources, "resources/PressStart2P-Regular.ttf", nil)
	audioContext        = audio.NewContext(48000)
	gameStartAudioData  = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂 効果音 システム49.mp3.dat", audioContext)
	gameOverAudioData   = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂 効果音 システム32.mp3.dat", audioContext)
	scoreUpAudioData    = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂 効果音 物音15.mp3.dat", audioContext)
	waveAudioData       = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂  水02.mp3.dat", audioContext)
	bgmPlayer           = resourceutil.ForceCreateBGMPlayer(resources, "resources/bgm-wave-interference.wav", audioContext)
)

type wave struct {
	x      float64
	vx     float64
	height float64
	length float64
}

type coin struct {
	x, y float64
	vx   float64
	r    float64
}

var coinImage = dotutil.CreatePatternImage([][]int{
	{0, 0, 1, 1, 0, 0},
	{0, 1, 3, 1, 1, 0},
	{1, 3, 1, 1, 1, 1},
	{1, 1, 1, 1, 2, 1},
	{1, 1, 1, 1, 2, 1},
	{1, 1, 1, 1, 2, 1},
	{1, 1, 1, 1, 2, 1},
	{0, 1, 1, 2, 1, 0},
	{0, 0, 1, 1, 0, 0},
}, &dotutil.CreatePatternImageOption{
	ColorMap: map[int]color.Color{
		1: color.RGBA{0xff, 0xe0, 0, 0xff},
		2: color.RGBA{0xf5, 0xc0, 0, 0xff},
		3: color.RGBA{0xff, 0xf0, 0, 0xff},
	},
})

func (c *coin) draw(screen *ebiten.Image, game *Game) {
	_, h := coinImage.Size()
	dotutil.DrawImage(screen, coinImage, c.x, c.y, &dotutil.DrawImageOption{
		Scale:        c.r * 2 / float64(h),
		BasePosition: dotutil.DrawImagePositionCenter,
	})
}

type coinEffect struct {
	x, y  float64
	plus  int
	ticks uint
}

func (e *coinEffect) draw(screen *ebiten.Image, game *Game) {
	t := fmt.Sprintf("+%d", e.plus)
	y := e.y - 30*math.Sin(float64(e.ticks)/60*math.Pi)
	text.Draw(screen, t, fontM.Face, int(e.x), int(y), color.RGBA{0xff, 0xe0, 0, 0xff})
}

type shark struct {
	x, y      float64
	vx, vy    float64
	r         float64
	attacking bool
}

var sharkImage = dotutil.CreatePatternImage([][]int{
	{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0},
	{1, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0},
	{1, 1, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
	{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
	{0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	{0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
	{1, 1, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
	{1, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
}, &dotutil.CreatePatternImageOption{
	ColorMap: map[int]color.Color{
		1: color.Black,
	},
})

func (s *shark) draw(screen *ebiten.Image, game *Game) {
	_, h := coinImage.Size()
	scaleX := s.r * 2 / float64(h)
	scaleY := scaleX
	rotate := math.Atan2(s.vy, s.vx)
	if s.vx < 0 {
		scaleX *= -1
		rotate += math.Pi
	}
	dotutil.DrawImage(screen, sharkImage, s.x, s.y, &dotutil.DrawImageOption{
		ScaleX:       scaleX,
		ScaleY:       scaleY,
		Rotate:       rotate,
		BasePosition: dotutil.DrawImagePositionCenter,
	})
}

type splashEffect struct {
	x, y   float64
	vx, vy float64
	ticks  uint
}

func (e *splashEffect) draw(screen *ebiten.Image, game *Game) {
	ebitenutil.DrawRect(screen, e.x-1.5, e.y-1.5, 3, 3, color.RGBA{0, 0, 0xff, 0xff})
}

type gameMode int

const (
	gameModeTitle gameMode = iota
	gameModePlaying
	gameModeGameOver
)

type Game struct {
	playID             string
	mode               gameMode
	touchContext       *touchutil.TouchContext
	hold               bool
	ticksFromModeStart uint64
	score              int
	waves              []wave
	coins              []coin
	sharks             []shark
	coinEffects        []coinEffect
	splashEffects      []splashEffect
	nextWaveHeight     float64
	humanHeight        float64
}

func (g *Game) Update() error {
	g.touchContext.Update()

	g.ticksFromModeStart++

	switch g.mode {
	case gameModeTitle:
		if g.touchContext.IsJustTouched() {
			g.mode = gameModePlaying
			g.ticksFromModeStart = 0
			g.waves = nil
			g.humanHeight = 0

			logging.LogAsync(gameName, map[string]interface{}{
				"play_id": g.playID,
				"action":  "start_game",
			})

			audio.NewPlayerFromBytes(audioContext, gameStartAudioData).Play()

			bgmPlayer.Rewind()
			bgmPlayer.Play()
		}
	case gameModePlaying:
		if g.ticksFromModeStart%600 == 0 {
			logging.LogAsync(gameName, map[string]interface{}{
				"play_id": g.playID,
				"action":  "playing",
				"ticks":   g.ticksFromModeStart,
				"score":   g.score,
			})
		}

		if g.hold {
			g.nextWaveHeight += 0.8
			if g.nextWaveHeight > 300 {
				g.nextWaveHeight = 300
			}
		}

		if g.touchContext.IsJustTouched() {
			g.hold = true
			g.nextWaveHeight = 0
		}
		if g.touchContext.IsJustReleased() {
			if g.hold {
				w := wave{
					x:      -100,
					vx:     waveSpeed,
					height: g.nextWaveHeight,
					length: 200,
				}
				g.waves = append(g.waves, w)

				if g.nextWaveHeight > 50 {
					audio.NewPlayerFromBytes(audioContext, waveAudioData).Play()
				}
			}

			g.hold = false
			g.nextWaveHeight = 0
		}

		// Wave enter
		if g.ticksFromModeStart%180 == 0 {
			w := wave{
				x:      screenWidth + 75,
				vx:     -waveSpeed,
				height: 50,
				length: 150,
			}
			g.waves = append(g.waves, w)
		}

		// Coin enter
		if g.ticksFromModeStart%180 == 0 {
			var x, vx float64
			if rand.Int()%2 == 0 {
				x = -50
				vx = coinSpeed
			} else {
				x = screenWidth + 50
				vx = -coinSpeed
			}
			c := coin{
				x:  x,
				y:  (surfaceBaseHeight-200)*rand.Float64() + 50,
				vx: vx,
				r:  coinR,
			}
			g.coins = append(g.coins, c)
		}

		// Shark enter
		if g.ticksFromModeStart%630 == 0 {
			var x, vx float64
			if rand.Int()%2 == 0 {
				x = -50
				vx = sharkSpeed
			} else {
				x = screenWidth + 50
				vx = -sharkSpeed
			}
			s := shark{
				x:  x,
				y:  surfaceBaseHeight + 100,
				vx: vx,
				vy: 0,
				r:  sharkR,
			}
			g.sharks = append(g.sharks, s)
		}

		// Wave move
		var newWaves []wave
		for i := 0; i < len(g.waves); i++ {
			w := &g.waves[i]
			w.x += w.vx

			if w.x > -w.length && w.x < screenWidth+w.length {
				newWaves = append(newWaves, *w)
			}
		}
		g.waves = newWaves

		// Coin move
		var newCoins []coin
		for i := 0; i < len(g.coins); i++ {
			c := &g.coins[i]
			c.x += c.vx

			if c.x > -50 && c.x < screenWidth+50 {
				newCoins = append(newCoins, *c)
			}
		}
		g.coins = newCoins

		// Shark move
		var newSharks []shark
		for i := 0; i < len(g.sharks); i++ {
			s := &g.sharks[i]

			d2 := math.Pow(screenCenterX-s.x, 2) + math.Pow(surfaceBaseHeight-s.y, 2)
			if d2 < math.Pow(150, 2) {
				if s.vx > 0 && s.x < screenCenterX || s.vx < 0 && s.x > screenCenterX {
					s.attacking = true
				}
			}
			if d2 < math.Pow(s.r, 2) {
				s.attacking = false
				s.vy = 0
				if s.vx > 0 {
					s.vx = sharkSpeed
				} else {
					s.vx = -sharkSpeed
				}
			}

			if s.attacking {
				t := math.Atan2(surfaceBaseHeight-s.y, screenCenterX-s.x)
				s.vx = sharkSpeed * 2.5 * math.Cos(t)
				s.vy = sharkSpeed * 2.5 * math.Sin(t)
			}

			s.x += s.vx
			s.y += s.vy

			if s.x > -100 && s.x < screenWidth+100 {
				newSharks = append(newSharks, *s)
			}
		}
		g.sharks = newSharks

		prevHumanHeight := g.humanHeight
		g.humanHeight = g.calcSurfaceHeight(screenCenterX)

		// Splash effect enter
		if g.humanHeight == 0 && prevHumanHeight > 0 {
			for i := 0; i < 10; i++ {
				t := (math.Pi/4 + math.Pi/2*rand.Float64()) * -1
				v := 4 * math.Log(prevHumanHeight)
				g.splashEffects = append(g.splashEffects, splashEffect{
					x:  screenCenterX,
					y:  surfaceBaseHeight,
					vx: v * math.Cos(t),
					vy: v * math.Sin(t),
				})
			}
		}

		// Coin effect move
		var newCoinEffects []coinEffect
		for i := 0; i < len(g.coinEffects); i++ {
			e := &g.coinEffects[i]
			e.ticks++
			if e.ticks < 60 {
				newCoinEffects = append(newCoinEffects, *e)
			}
		}
		g.coinEffects = newCoinEffects

		// Splash effect move
		var newSplashEffects []splashEffect
		for i := 0; i < len(g.splashEffects); i++ {
			e := &g.splashEffects[i]
			e.ticks++
			e.vy += 0.1
			e.x += e.vx
			e.y += e.vy
			if e.y < surfaceBaseHeight {
				newSplashEffects = append(newSplashEffects, *e)
			}
		}
		g.splashEffects = newSplashEffects

		// Coins and human collision
		newCoins = []coin{}
		for i := 0; i < len(g.coins); i++ {
			c := &g.coins[i]
			if math.Pow(c.x-screenCenterX, 2)+math.Pow(c.y-(surfaceBaseHeight-g.humanHeight), 2) < math.Pow(c.r+humanR, 2) {
				var plus int
				if g.humanHeight > 300 {
					plus = 4
				} else if g.humanHeight > 250 {
					plus = 3
				} else if g.humanHeight > 200 {
					plus = 2
				} else {
					plus = 1
				}

				g.score += plus

				g.coinEffects = append(g.coinEffects, coinEffect{
					x:    c.x,
					y:    c.y,
					plus: plus,
				})

				audio.NewPlayerFromBytes(audioContext, scoreUpAudioData).Play()
			} else {
				newCoins = append(newCoins, *c)
			}
		}
		g.coins = newCoins

		// Sharks and human collision
		for i := 0; i < len(g.sharks); i++ {
			s := &g.sharks[i]
			if math.Pow(s.x-screenCenterX, 2)+math.Pow(s.y-(surfaceBaseHeight-g.humanHeight), 2) < math.Pow(s.r+humanR, 2) {
				logging.LogAsync(gameName, map[string]interface{}{
					"play_id": g.playID,
					"action":  "game_over",
					"ticks":   g.ticksFromModeStart,
					"score":   g.score,
				})

				g.mode = gameModeGameOver
				g.ticksFromModeStart = 0

				audio.NewPlayerFromBytes(audioContext, gameOverAudioData).Play()
			}
		}
	case gameModeGameOver:
		if g.ticksFromModeStart > 15 && g.touchContext.IsJustTouched() {
			g.initialize()
			bgmPlayer.Pause()
		}
	}

	return nil
}

func (g *Game) calcSurfaceHeight(x float64) float64 {
	var h float64
	for _, w := range g.waves {
		if math.Abs(x-w.x) < w.length/2 {
			theta := math.Pi * (x - (w.x - w.length/2)) / w.length
			h += w.height * math.Sin(theta)
		}
	}
	return h
}

func (g *Game) drawSurface(screen *ebiten.Image) {
	x := 0.0
	for x < screenWidth {
		y := surfaceBaseHeight - g.calcSurfaceHeight(x)
		ebitenutil.DrawRect(screen, x, y, 5, 5, color.RGBA{0, 0, 0xff, 0xff})
		x += 5 + 3
	}

	ebitenutil.DrawRect(screen, 3, surfaceBaseHeight-g.nextWaveHeight, 5, g.nextWaveHeight, color.RGBA{0, 0, 0xff, 0xff})
}

var humanImage = dotutil.CreatePatternImage([][]int{
	{0, 0, 2, 2, 2, 2, 0, 0},
	{0, 2, 2, 2, 2, 2, 2, 0},
	{2, 3, 3, 2, 3, 3, 2, 2},
	{2, 3, 4, 2, 4, 3, 2, 2},
	{2, 2, 2, 2, 2, 2, 2, 2},
	{2, 2, 2, 2, 2, 2, 2, 2},
	{0, 2, 2, 2, 2, 2, 2, 0},
	{1, 1, 1, 1, 1, 1, 1, 1},
	{0, 1, 1, 1, 1, 1, 1, 0},
}, &dotutil.CreatePatternImageOption{
	ColorMap: map[int]color.Color{
		1: color.Black,
		2: color.RGBA{0xff, 0, 0, 0xff},
		3: color.White,
		4: color.Black,
	},
})

func (g *Game) drawHuman(screen *ebiten.Image) {
	_, h := humanImage.Size()
	dotutil.DrawImage(screen, humanImage, screenCenterX, surfaceBaseHeight-g.humanHeight, &dotutil.DrawImageOption{
		Scale:        humanR * 2 / float64(h),
		BasePosition: dotutil.DrawImagePositionCenter,
	})
}

func (g *Game) drawScore(screen *ebiten.Image) {
	scoreText := fmt.Sprintf("SCORE %d", g.score)
	text.Draw(screen, scoreText, fontS.Face, screenWidth-(len(scoreText)+1)*int(fontS.FaceOptions.Size), 20, color.Black)
}

func (g *Game) drawTitle(screen *ebiten.Image) {
	titleText := []string{"WAVE INTERFERENCE"}
	for i, s := range titleText {
		text.Draw(screen, s, fontL.Face, screenCenterX-len(s)*int(fontL.FaceOptions.Size)/2, 110+i*int(fontL.FaceOptions.Size*1.8), color.RGBA{0, 0, 0xff, 0xff})
	}

	usageTexts := []string{"[HOLD] Charge Wave", "[RELEASE] Release Wave"}
	for i, s := range usageTexts {
		text.Draw(screen, s, fontS.Face, screenCenterX-len(s)*int(fontS.FaceOptions.Size)/2, 160+i*int(fontS.FaceOptions.Size*1.8), color.Black)
	}

	creditTexts := []string{"CREATOR: NAOKI TSUJIO", "FONT: Press Start 2P by CodeMan38", "SOUND EFFECT: MaouDamashii"}
	for i, s := range creditTexts {
		text.Draw(screen, s, fontS.Face, screenCenterX-len(s)*int(fontS.FaceOptions.Size)/2, 400+i*int(fontS.FaceOptions.Size*1.8), color.Black)
	}
}

func (g *Game) drawGameOver(screen *ebiten.Image) {
	gameOverText := "GAME OVER"
	text.Draw(screen, gameOverText, fontL.Face, screenCenterX-len(gameOverText)*int(fontL.FaceOptions.Size)/2, 130, color.Black)
	scoreText := []string{"YOUR SCORE IS", fmt.Sprintf("%d!", g.score)}
	for i, s := range scoreText {
		text.Draw(screen, s, fontM.Face, screenCenterX-len(s)*int(fontM.FaceOptions.Size)/2, 210+i*int(fontM.FaceOptions.Size*2), color.Black)
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0xef, 0xef, 0xef, 0xff})

	switch g.mode {
	case gameModeTitle:
		g.drawSurface(screen)

		(&shark{x: 70, y: surfaceBaseHeight + 50, vx: 1, r: sharkR}).draw(screen, g)
		(&shark{x: screenWidth - 80, y: surfaceBaseHeight + 100, vx: -1, r: sharkR}).draw(screen, g)

		g.drawHuman(screen)
		g.drawTitle(screen)
	case gameModePlaying:
		for i := 0; i < len(g.sharks); i++ {
			g.sharks[i].draw(screen, g)
		}
		g.drawSurface(screen)
		for i := 0; i < len(g.coins); i++ {
			g.coins[i].draw(screen, g)
		}
		for i := 0; i < len(g.coinEffects); i++ {
			g.coinEffects[i].draw(screen, g)
		}
		for i := 0; i < len(g.splashEffects); i++ {
			g.splashEffects[i].draw(screen, g)
		}
		g.drawHuman(screen)
		g.drawScore(screen)
	case gameModeGameOver:
		g.drawSurface(screen)
		for i := 0; i < len(g.sharks); i++ {
			g.sharks[i].draw(screen, g)
		}
		for i := 0; i < len(g.coins); i++ {
			g.coins[i].draw(screen, g)
		}
		for i := 0; i < len(g.coinEffects); i++ {
			g.coinEffects[i].draw(screen, g)
		}
		for i := 0; i < len(g.splashEffects); i++ {
			g.splashEffects[i].draw(screen, g)
		}
		g.drawHuman(screen)
		g.drawScore(screen)
		g.drawGameOver(screen)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) initialize() {
	logging.LogAsync(gameName, map[string]interface{}{
		"play_id": g.playID,
		"action":  "initialize",
	})

	g.mode = gameModeTitle
	g.ticksFromModeStart = 0
	g.hold = false
	g.score = 0
	g.waves = nil
	g.coins = nil
	g.sharks = nil
	g.coinEffects = nil
	g.splashEffects = nil
	g.humanHeight = 0
	g.nextWaveHeight = 0

	// For title drawing
	g.waves = []wave{
		{x: 200, height: 60, length: 100},
		{x: 320, height: 90, length: 100},
		{x: 440, height: 60, length: 100},
	}
	g.humanHeight = g.calcSurfaceHeight(screenCenterX)
}

func main() {
	if os.Getenv("GAME_LOGGING") != "1" {
		logging.Disable()
	}
	if seed, err := strconv.Atoi(os.Getenv("GAME_RAND_SEED")); err == nil {
		rand.Seed(int64(seed))
	} else {
		rand.Seed(time.Now().Unix())
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Wave Interference")

	playIDObj, err := uuid.NewRandom()
	var playID string
	if err != nil {
		playID = "?"
	} else {
		playID = playIDObj.String()
	}

	game := &Game{
		playID:       playID,
		touchContext: touchutil.CreateTouchContext(),
	}
	game.initialize()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
