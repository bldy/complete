// Copyright 2017 Sevki <s@sevki.org>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main // import "bldy.build/complete"
import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"9fans.net/go/acme"
	"bldy.build/build/blaze"
	"bldy.build/build/project"
	"bldy.build/build/url"
	"github.com/go-clang/v3.8/clang"

	"bldy.build/build/targets/cc"
)

var (
	win    *acme.Win
	active *acme.Win
	events <-chan *acme.Event
	ltok   = regexp.MustCompile("[[:alnum:]]*$")
)

func main() {

	go listenToAcmeEvents()
	active = nil
	select {}
}
func gl(strs []string) string {
	if len(strs) > 0 {
		return strs[len(strs)-1]
	}
	return ""
}
func listenToAcmeEvents() {
	l, err := acme.Log()
	if err != nil {
		log.Print(err)
	}
	for {
		event, err := l.Read()
		if err != nil {
			log.Print(err)
		}
		if active != nil {
			active.CloseFiles()
		}
		if filepath.Ext(event.Name) == ".c" {
			active, err = acme.Open(event.ID, nil)
			if err != nil {
				log.Println(err)
			}

			idx := clang.NewIndex(0, 1)
			defer idx.Dispose()
			dir, _ := filepath.Split(event.Name)
			dirs := strings.Split(dir, "/")
			var try string
			for i := len(dirs) - 1; i > 0; i-- {
				try = fmt.Sprintf("/%s/BUILD", filepath.Join(dirs[0:i+1]...))
				if _, err := os.Lstat(try); os.IsNotExist(err) {
					continue
				} else if err != nil {
					log.Fatal(err)
				}
				break
			}
			wd, _ := filepath.Split(try)
			bvm := blaze.NewVM(wd)
			root := project.GetGitDir(wd)
			project.SideLoad(root)
			rel, _ := filepath.Rel(root, wd)
			_, targ := filepath.Split(rel)
			u := url.URL{
				Target:  targ,
				Package: rel,
			}
			t, err := bvm.GetTarget(u)
			if err != nil {
				log.Fatal(err)
			}
			flags := []string{}
			switch t.(type) {
			case *cc.CLib:
				ccbin := t.(*cc.CLib)
				flags = append(flags, []string(ccbin.CompilerOptions)...)
				for _, inc := range ccbin.Includes {
					a := filepath.Join(root, inc)
					flags = append(flags, fmt.Sprintf("-I%s", a))
				}
				flags = append(flags, ccbin.LinkerOptions...)
			case *cc.CBin:
				ccbin := t.(*cc.CBin)
				flags = append(flags, []string(ccbin.CompilerOptions)...)
				for _, inc := range ccbin.Includes {
					a := filepath.Join(root, inc)
					flags = append(flags, fmt.Sprintf("-I%s", a))
				}
				flags = append(flags, ccbin.LinkerOptions...)
			}
			tu := idx.ParseTranslationUnit(event.Name, flags, nil, 0)
			defer tu.Dispose()
			win, err := acme.New()
			if err != nil {
				log.Fatal(err)
			}
			win.Name("+complete")
			win.Ctl("clean")
			for {
				select {
				case ev := <-active.EventChan():
					bytz, err := active.ReadAll("body")
					if err != nil {
						log.Fatal(err)
					}
					win.Seek("body", 0, 0)
					lines := strings.Split(string(bytz[:ev.Q1]), "\n")
					l := uint32(len(lines))
					lastLine := lines[l-1]
					c := uint32(len(lastLine))

					lastToken := gl(ltok.FindAllString(lastLine, -1))
					n := clang.NewUnsavedFile(event.Name, string(bytz))
					if lastToken == "" {
						continue
					}
					ccr := tu.CodeCompleteAt(event.Name, l, c, []clang.UnsavedFile{n}, 0).Results()
					clang.SortCodeCompletionResults(ccr)
					file := tu.File(event.Name)
					loc := tu.Location(file, l, 0)
					rng := loc.Range(tu.Location(file, l, c))
					tokens := tu.Tokenize(rng)
					for _, tok := range tokens {
						log.Println(tok.Kind().Spelling())
					}
					var i uint32
					j := 0
					for _, cc := range ccr {
						j++
						cs := cc.CompletionString()
						s := ""
						for i = 0; i < cs.NumChunks(); i++ {
							if cs.ChunkKind(i) != clang.CompletionChunk_TypedText {
								continue
							}

							s = cs.ChunkText(i)
						}

						if strings.HasPrefix(s, lastToken) {
							win.Write("body", []byte(fmt.Sprintf("%d ", j)))

							win.Write("body", []byte(fmt.Sprintf("%s", s)))

							win.Write("body", []byte(fmt.Sprintln()))
						}
					}
				}
			}
		}
	}
}
