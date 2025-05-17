package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Language описывает структуру языка для загрузки из JSON-конфига.
type Language struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Extensions []string `json:"extensions"`
}

// AuthorStats хранит статистику по конкретному автору (строки, коммиты, файлы).
type AuthorStats struct {
	Name    string
	Lines   int
	Commits map[string]struct{}
	Files   map[string]struct{}
}

// getLanguagesExtensions читает конфигурационный файл, путь к которому передаётся через параметр,
// и добавляет в общий список extensions те расширения, которые соответствуют указанным языкам.
func getLanguagesExtensions(languages []string, extensions *[]string, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл конфигурации: %v", err)
	}

	var langs []Language
	if err := json.Unmarshal(data, &langs); err != nil {
		return fmt.Errorf("ошибка разбора JSON: %v", err)
	}

	// Создаем map для быстрого поиска языка по имени (в нижнем регистре)
	langMap := make(map[string]Language, len(langs))
	for _, lang := range langs {
		langMap[strings.ToLower(lang.Name)] = lang
	}

	// Для каждого языка, указанного пользователем, добавляем все его расширения
	for _, langName := range languages {
		lowerLang := strings.ToLower(langName)
		if langDef, ok := langMap[lowerLang]; ok {
			*extensions = append(*extensions, langDef.Extensions...)
		}
	}
	return nil
}

// filterExtension отбирает только те файлы, у которых расширение есть в списке.
func filterExtension(files []string, extensions []string) []string {
	extSet := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		extSet[strings.ToLower(ext)] = struct{}{}
	}

	var filtered []string
	for _, file := range files {
		fileExt := strings.ToLower(filepath.Ext(file))
		if _, ok := extSet[fileExt]; ok {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// isFileEmpty проверяет, что файл существует и имеет размер 0.
func isFileEmpty(repoPath, file string) bool {
	info, err := os.Stat(filepath.Join(repoPath, file))
	if err != nil {
		return false
	}
	return info.Size() == 0
}

// blameEmptyFile обрабатывает случай, когда файл пустой.
func blameEmptyFile(repoPath, revision, file string, useCommitter bool) (map[string]*AuthorStats, error) {
	cmdHash := exec.Command("git", "-C", repoPath, "log", revision, "-1", "--format=%H", "--", file)
	hashBytes, err := cmdHash.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %v", err)
	}
	commitHash := strings.TrimSpace(string(hashBytes))

	formatArg := "%an"
	if useCommitter {
		formatArg = "%cn"
	}
	cmdAuthor := exec.Command("git", "-C", repoPath, "log", revision, "-1", "--format="+formatArg, "--", file)
	authorBytes, err := cmdAuthor.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed (author): %v", err)
	}
	author := strings.TrimSpace(string(authorBytes))

	stats := make(map[string]*AuthorStats)
	stats[author] = &AuthorStats{
		Name:    author,
		Lines:   0,
		Commits: map[string]struct{}{commitHash: {}},
		Files:   map[string]struct{}{file: {}},
	}
	return stats, nil
}

// blameFile возвращает статистику по строкам для каждого автора файла.
func blameFile(repoPath, revision, filePath string, useCommitter bool) (map[string]*AuthorStats, error) {
	commitToAuthor := make(map[string]string)
	cmd := exec.Command("git", "-C", repoPath, "blame", "--porcelain", "-l", revision, "--", filePath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ошибка git blame: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	stats := make(map[string]*AuthorStats)

	var block []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		if line[0] == '\t' {
			if len(block) == 0 {
				continue
			}
			headerFields := strings.Fields(block[0])
			commit := headerFields[0]
			var author string

			for _, bline := range block {
				if !useCommitter && strings.HasPrefix(bline, "author ") {
					author = strings.TrimSpace(bline[len("author "):])
					break
				}
				if useCommitter && strings.HasPrefix(bline, "committer ") {
					author = strings.TrimSpace(strings.Join(strings.Fields(bline)[1:], " "))
					break
				}
			}
			if author == "" {
				author = commitToAuthor[commit]
			} else {
				commitToAuthor[commit] = author
			}

			stat, ok := stats[author]
			if !ok {
				stat = &AuthorStats{
					Name:    author,
					Commits: make(map[string]struct{}),
					Files:   make(map[string]struct{}),
				}
				stats[author] = stat
			}
			stat.Lines++
			stat.Commits[commit] = struct{}{}
			stat.Files[filePath] = struct{}{}

			block = nil
			continue
		}
		block = append(block, line)
	}

	return stats, nil
}

// filterByGlob позволяет исключать или включать файлы по glob-паттернам.
func filterByGlob(files []string, patterns []string, include bool) []string {
	var filtered []string
	for _, file := range files {
		matched := false
		for _, pattern := range patterns {
			ok, err := filepath.Match(pattern, file)
			if err == nil && ok {
				matched = true
				break
			}
		}
		if (include && matched) || (!include && !matched) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// getGitFiles возвращает список файлов в репозитории по заданной ревизии.
func getGitFiles(repoPath, revision string) ([]string, error) {
	revCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", revision)
	if err := revCmd.Run(); err != nil {
		return nil, fmt.Errorf("неверная ревизия (%s): %v", revision, err)
	}
	cmd := exec.Command("git", "-C", repoPath, "ls-tree", "-r", "--name-only", revision)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-tree failed: %v", err)
	}
	rawLines := strings.Split(out.String(), "\n")
	var lines []string
	for _, line := range rawLines {
		if trimmed := strings.TrimSpace(line); len(trimmed) > 0 {
			lines = append(lines, trimmed)
		}
	}
	return lines, nil
}

func main() {
	repoPath := flag.String("repository", ".", "Путь к git-репозиторию")
	revision := flag.String("revision", "HEAD", "Коммит или ветка")
	orderBy := flag.String("order-by", "lines", "Сортировка: lines, commits или files")
	useCommitter := flag.Bool("use-committer", false, "Использовать коммиттера вместо автора")
	format := flag.String("format", "tabular", "Формат вывода: tabular, csv, json, json-lines")

	// Параметры для фильтрации
	extensions := flag.String("extensions", "", "Файловые расширения, например '.go,.md'")
	languages := flag.String("languages", "", "Список языков, например 'go,markdown'")
	exclude := flag.String("exclude", "", "Glob-паттерны для исключения файлов, например 'foo/*,bar/*'")
	restrictTo := flag.String("restrict-to", "", "Glob-паттерны для включения файлов, игнорировать остальные")

	// Путь к конфигурационному файлу с расширениями языков.
	languageConfigPath := flag.String("languages-config-path", "../../configs/language_extensions.json", "Путь к файлу с расширениями языков (JSON)")

	flag.Parse()

	// Проверяем корректность параметра order-by.
	validOrderBy := map[string]bool{
		"lines":   true,
		"commits": true,
		"files":   true,
	}
	if !validOrderBy[*orderBy] {
		fmt.Fprintf(os.Stderr, "ошибка: --order-by может быть только lines, commits или files, но получено: %q\n", *orderBy)
		os.Exit(1)
	}

	// Проверяем корректность формата вывода.
	switch *format {
	case "tabular", "csv", "json", "json-lines":
	default:
		fmt.Fprintf(os.Stderr, "ошибка: неизвестный формат вывода: %q. Допустимые значения: tabular, csv, json, json-lines\n", *format)
		os.Exit(1)
	}

	// Получаем список файлов из git.
	files, err := getGitFiles(*repoPath, *revision)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка получения файлов из git:", err)
		os.Exit(1)
	}
	var validFiles []string
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file != "" {
			validFiles = append(validFiles, file)
		}
	}
	files = validFiles

	// Разбираем параметры languages, exclude, restrict.
	var langsList, excludeList, restrictList []string
	if *languages != "" {
		langsList = strings.Split(*languages, ",")
	}
	if *exclude != "" {
		excludeList = strings.Split(*exclude, ",")
	}
	if *restrictTo != "" {
		restrictList = strings.Split(*restrictTo, ",")
	}

	// Разбираем расширения, переданные через --extensions.
	var extList []string
	if *extensions != "" {
		extList = strings.Split(*extensions, ",")
	}

	// Если заданы языки, читаем конфигурацию для получения расширений.
	if len(langsList) > 0 {
		if err = getLanguagesExtensions(langsList, &extList, *languageConfigPath); err != nil {
			fmt.Fprintf(os.Stderr, "ошибка получения расширений по языкам: %v\n", err)
			os.Exit(1)
		}
	}

	// Если есть расширения, фильтруем файлы.
	if len(extList) > 0 {
		files = filterExtension(files, extList)
	}

	// Применяем фильтрацию по исключению (exclude) и ограничению (restrict).
	if len(excludeList) > 0 {
		files = filterByGlob(files, excludeList, false)
	}
	if len(restrictList) > 0 {
		files = filterByGlob(files, restrictList, true)
	}

	// Сбор общей статистики по всем авторам.
	totalStats := make(map[string]*AuthorStats)
	for _, file := range files {
		var fileStats map[string]*AuthorStats

		if isFileEmpty(*repoPath, file) {
			fileStats, err = blameEmptyFile(*repoPath, *revision, file, *useCommitter)
		} else {
			fileStats, err = blameFile(*repoPath, *revision, file, *useCommitter)
			if err == nil && len(fileStats) == 0 {
				fileStats, err = blameEmptyFile(*repoPath, *revision, file, *useCommitter)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "ошибка анализа файла %s: %v\n", file, err)
			continue
		}

		for name, fileStat := range fileStats {
			stat, ok := totalStats[name]
			if !ok {
				stat = &AuthorStats{
					Name:    name,
					Commits: make(map[string]struct{}),
					Files:   make(map[string]struct{}),
				}
				totalStats[name] = stat
			}
			stat.Lines += fileStat.Lines
			for commit := range fileStat.Commits {
				stat.Commits[commit] = struct{}{}
			}
			for f := range fileStat.Files {
				stat.Files[f] = struct{}{}
			}
		}
	}

	var authors []*AuthorStats
	for _, stat := range totalStats {
		authors = append(authors, stat)
	}

	sort.Slice(authors, func(i, j int) bool {
		a, b := authors[i], authors[j]
		switch *orderBy {
		case "lines":
			if a.Lines != b.Lines {
				return a.Lines > b.Lines
			}
			if len(a.Commits) != len(b.Commits) {
				return len(a.Commits) > len(b.Commits)
			}
			if len(a.Files) != len(b.Files) {
				return len(a.Files) > len(b.Files)
			}
		case "commits":
			if len(a.Commits) != len(b.Commits) {
				return len(a.Commits) > len(b.Commits)
			}
			if a.Lines != b.Lines {
				return a.Lines > b.Lines
			}
			if len(a.Files) != len(b.Files) {
				return len(a.Files) > len(b.Files)
			}
		case "files":
			if len(a.Files) != len(b.Files) {
				return len(a.Files) > len(b.Files)
			}
			if a.Lines != b.Lines {
				return a.Lines > b.Lines
			}
			if len(a.Commits) != len(b.Commits) {
				return len(a.Commits) > len(b.Commits)
			}
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	// Вывод результата.
	switch *format {
	case "tabular":
		fmt.Printf("%-23s%-5s %-8s%s\n", "Name", "Lines", "Commits", "Files")
		for _, stat := range authors {
			fmt.Printf("%-23s%-5d %-8d%d\n", stat.Name, stat.Lines, len(stat.Commits), len(stat.Files))
		}
	case "csv":
		writer := csv.NewWriter(os.Stdout)
		if err := writer.Write([]string{"Name", "Lines", "Commits", "Files"}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write CSV header: %v\n", err)
			os.Exit(1)
		}
		for _, stat := range authors {
			record := []string{
				stat.Name,
				strconv.Itoa(stat.Lines),
				strconv.Itoa(len(stat.Commits)),
				strconv.Itoa(len(stat.Files)),
			}
			if err := writer.Write(record); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write CSV record: %v\n", err)
				os.Exit(1)
			}
		}
		writer.Flush()
	case "json":
		type jsonAuthor struct {
			Name    string `json:"name"`
			Lines   int    `json:"lines"`
			Commits int    `json:"commits"`
			Files   int    `json:"files"`
		}
		var jsonData []jsonAuthor
		for _, stat := range authors {
			jsonData = append(jsonData, jsonAuthor{
				Name:    stat.Name,
				Lines:   stat.Lines,
				Commits: len(stat.Commits),
				Files:   len(stat.Files),
			})
		}
		data, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка сериализации JSON:", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	case "json-lines":
		type jsonAuthor struct {
			Name    string `json:"name"`
			Lines   int    `json:"lines"`
			Commits int    `json:"commits"`
			Files   int    `json:"files"`
		}
		for _, stat := range authors {
			j := jsonAuthor{
				Name:    stat.Name,
				Lines:   stat.Lines,
				Commits: len(stat.Commits),
				Files:   len(stat.Files),
			}
			data, err := json.Marshal(j)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ошибка сериализации JSON:", err)
				os.Exit(1)
			}
			fmt.Println(string(data))
		}
	}
}
