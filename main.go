package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net"
	"os"
	"sync"
	"time"
)


type size struct {
	width  int
	height int
}

type chunk struct {
	xPos   int
	width  int
	height int
	scale  float64
}

func getSize(conn net.Conn) *size {

	var canvasSize size
	conn.Write([]byte("SIZE\n"))
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		fmt.Println("Could not get size: ", err)
		return nil
	}

	fmt.Sscanf(reply, "SIZE %d %d", &canvasSize.width, &canvasSize.height)
	return &canvasSize
}

func writePixel(x, y, r, g, b, a int, conn net.Conn) {
	var cmd string
	if a == 255 {
		cmd = fmt.Sprintf("PX %d %d %02x%02x%02x\n", x, y, r, g, b)
	} else {
		cmd = fmt.Sprintf("PX %d %d %02x%02x%02x%02x\n", x, y, r, g, b, a)
	}
	conn.Write([]byte(cmd))
}

func drawCircle(x, y, rad, r, g, b int, conn net.Conn) {
	for j := 0; j < rad; j++ {
		for i := 0; i < rad; i++ {
			if (i*i + j*j) < rad*rad {
				writePixel(x-i, y-j, r, g, b, 255, conn)
				writePixel(x+i, y+j, r, g, b, 255, conn)
				writePixel(x-i, y+j, r, g, b, 255, conn)
				writePixel(x+i, y-j, r, g, b, 255, conn)
			}
		}
	}
}

func drawRect(x, y, w, h, r, g, b int, conn net.Conn) {
	for i := x; i < x+h; i++ {
		for j := y; j < y+w; j++ {
			writePixel(j, i, r, g, b, 255, conn)
		}
	}
}

func bouncingBall(x, y, xvel, yvel, rad, r, g, b int, conn net.Conn) {
	for true {
		x += xvel
		y += yvel

		if x > 1280 || x < 0 {
			xvel *= -1
		}
		if y > 720 || y < 0 {
			yvel *= -1
		}

		drawCircle(x-xvel, y-yvel, rad, 255, 255, 255, conn)
		drawCircle(x, y, rad, r, g, b, conn)
	}
}

func bouncingImage(x, y, xvel, yvel int, path string, conn net.Conn, threads int) {
    f, err := os.Open(path)
    if err != nil {
        fmt.Fprintln(os.Stderr, "Can't open image womp womp")
        os.Exit(1)
    }
    
    img,  _, err  := image.Decode(f)
    bounds := img.Bounds()
    imgWidth := bounds.Max.X
    imgHeight := bounds.Max.Y

    for true {
        x += xvel
        y += yvel
        canvasSize := getSize(conn)

        if x > canvasSize.width || x < 0 {
            xvel *= -1
        }
        if y > canvasSize.height || y < 0{
            yvel *= -1
        }

        drawImage(path, x, y, threads, 0.1, conn)
        time.Sleep(50*time.Millisecond)
        drawRect(x-xvel, y-yvel, int(float64(imgWidth) * 0.1), int(float64(imgHeight) * 0.1), 255, 255, 255, conn)
    }
}
func makeChunks(threadsCount int, chunkWidth int, chunkHeight int, chunkScale float64) []chunk {

	chunks := make([]chunk, threadsCount)   // As many chunks as threads
	currIndex := 0
	for i := 0; i < len(chunks); i++{
		chunks[i] = chunk{
			xPos   : currIndex,
			width  : chunkWidth,
			height : chunkHeight,
			scale  : chunkScale,
		}

		currIndex += chunkWidth
	}

	return chunks
}

func drawChunk(chunk chunk, img image.Image, startX int, startY int, wg *sync.WaitGroup, conn net.Conn) {

	defer wg.Done()

	for x := chunk.xPos; x < (chunk.xPos + chunk.width); x++ {
		for y := 0; y <= chunk.height; y++ {
			scaledX := int(float64(x) / chunk.scale)
			scaledY := int(float64(y) / chunk.scale)

			r, g, b, a := img.At(scaledX, scaledY).RGBA()
			writePixel(x + startX, y + startY, int(r>>8), int(g>>8), int(b>>8), int(a>>8), conn)
		}
	}

}

func drawImage(path string, startX int, startY int, threads int, size float64, conn net.Conn) error {

	t0 := time.Now()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	imgWidth := bounds.Max.X
	imgHeight := bounds.Max.Y

	canvasSize := getSize(conn)

	// Calculate scaling factors
	widthScale := float64(canvasSize.width) / float64(imgWidth)
	heightScale := float64(canvasSize.height) / float64(imgHeight)
	scale := math.Min(widthScale, heightScale) * size

	// Calculate scaled dimensions
	scaledWidth := int(float64(imgWidth) * scale)
	scaledHeight := int(float64(imgHeight) * scale)

	chunkWidth := int(scaledWidth / threads)
	var chunks []chunk = makeChunks(threads, chunkWidth, scaledHeight, scale) 

	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go drawChunk(chunks[i], img, startX, startY, &wg, conn)
	}

	wg.Wait()
	fmt.Printf("drawImage runtime: %v\n", time.Since(t0))

	return nil
}

func main() {

	var host      *string = flag.String("host", "", "The PixelFlut server host ip or domain.")
	var port      *string = flag.String("port", "", "The port of the PixelFlut server.")
	var imagePath *string = flag.String("image", "", "The path to the image to draw.")
	var threads      *int = flag.Int("threads", 1, "Number of threads to use.")

	required := []string{"host", "port"}
	flag.Parse()

	seen := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	for _, req := range required {
		if !seen[req] {
			flag.Usage()
			os.Exit(2)
		}
	}

	if (*threads == 0) {
		fmt.Fprintln(os.Stderr, "Don't be silly")
		os.Exit(1)
	}

	connString := fmt.Sprintf("%s:%s", *host, *port)
	conn, err := net.Dial("tcp", connString)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not connect to \"" + connString + "\":\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	bouncingImage(0, 0, 1, 1, *imagePath, conn, *threads)

	// err = drawImage(*imagePath, 800, 0, *threads, 0.5, conn)
	// if err != nil {
	//     fmt.Fprintln(os.Stderr, "Could not draw image:" + "\n", err)
	//     os.Exit(1)
	// }
}
