// Command anpr-snapshot pulls still frames from a camera stream and sends each
// one to the NPDMS API for vehicle detection as a camera snapshot.
//
// It needs ffmpeg on the machine that can reach the camera (normally the edge
// server on the police network) and an access token for an officer of ASI
// rank or above; every snapshot is recorded against that officer with the
// stated purpose. Stream credentials are read from the environment, never
// from the command line, so they do not appear in the process list.
//
//	NPDMS_TOKEN=... ANPR_STREAM_URL='rtsp://user:pass@10.20.1.14:554/Streaming/Channels/101' \
//	  anpr-snapshot -api http://127.0.0.1:8080/api/v1 -camera <camera id> \
//	  -purpose "Junction watch for stolen vehicles, Gariahat" -interval 5s
//
// With -once it sends a single frame and exits. The API refuses snapshots
// while the vehicle detection module is switched off or its service is not
// connected, and this command reports that and keeps trying at the interval.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
)

func grab(ctx context.Context, source string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error"}
	if strings.HasPrefix(strings.ToLower(source), "rtsp://") {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args, "-i", source, "-frames:v", "1", "-q:v", "2", "-f", "image2", "-c:v", "mjpeg", "pipe:1")
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		// Never echo the stream URL: it can carry credentials.
		msg = strings.ReplaceAll(msg, source, "<stream>")
		return nil, fmt.Errorf("ffmpeg could not read a frame: %v %s", err, msg)
	}
	if out.Len() == 0 {
		return nil, errors.New("ffmpeg returned no frame")
	}
	return out.Bytes(), nil
}

func send(api, token, camera, purpose string, frame []byte, captured time.Time) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("cameraId", camera)
	_ = mw.WriteField("purpose", purpose)
	_ = mw.WriteField("capturedAt", captured.Format(time.RFC3339))
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="snapshot.jpg"`)
	h.Set("Content-Type", "image/jpeg")
	part, err := mw.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(frame); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(api, "/")+"/anpr/snapshots", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusCreated {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		return "", fmt.Errorf("API answered %s: %s", resp.Status, e.Message)
	}
	var a struct {
		AnalysisNumber string `json:"analysisNumber"`
		DetectionCount int    `json:"detectionCount"`
		PlateReadCount int    `json:"plateReadCount"`
		HitCount       int    `json:"hitCount"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %d vehicle(s), %d plate read(s), %d watchlist hit(s) pending review",
		a.AnalysisNumber, a.DetectionCount, a.PlateReadCount, a.HitCount), nil
}

func main() {
	api := flag.String("api", "http://127.0.0.1:8080/api/v1", "NPDMS API base URL")
	camera := flag.String("camera", "", "camera id from the camera register (required)")
	purpose := flag.String("purpose", "", "purpose recorded with every snapshot (required, 10+ characters)")
	interval := flag.Duration("interval", 10*time.Second, "time between snapshots")
	once := flag.Bool("once", false, "send one snapshot and exit")
	timeout := flag.Duration("grab-timeout", 20*time.Second, "time allowed to read one frame from the stream")
	flag.Parse()

	token := os.Getenv("NPDMS_TOKEN")
	source := os.Getenv("ANPR_STREAM_URL")
	if token == "" || source == "" || *camera == "" || len(strings.TrimSpace(*purpose)) < 10 {
		fmt.Fprintln(os.Stderr, "set NPDMS_TOKEN and ANPR_STREAM_URL, and give -camera and a -purpose of at least 10 characters")
		flag.Usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	for {
		captured := time.Now()
		frame, err := grab(ctx, source, *timeout)
		if err == nil {
			var summary string
			if summary, err = send(*api, token, *camera, *purpose, frame, captured); err == nil {
				log.Print(summary)
			}
		}
		if err != nil {
			log.Printf("snapshot not analysed: %v", err)
			if *once {
				os.Exit(1)
			}
		}
		if *once {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*interval):
		}
	}
}
