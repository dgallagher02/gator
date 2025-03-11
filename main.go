package main

import (
	"github.com/dgallagher02/gator_go/internal/config"
	"fmt"
	"os"
	"errors"
	"log"
	"github.com/dgallagher02/gator_go/internal/database"
	"time"
	"github.com/google/uuid"
	"context"
	"database/sql"
	"net/http"
	"io"
	"encoding/xml"
	"html"
	"strings"
	"strconv"
)

import _ "github.com/lib/pq"

// connection string to database - "postgres://postgres:postgres@localhost:5432/gator"

type state struct {
	db *database.Queries
	configPtr *config.Config
}

type command struct {
	name string
	args []string
}

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func unescape(item *RSSItem) {
	item.Title = html.UnescapeString(item.Title)
	item.Description = html.UnescapeString(item.Description)
}

func fetchFeed(ctx context.Context, feedUrl string) (*RSSFeed, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feedUrl, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	req.Header.Set("User-Agent", "gator")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch feed: status code %d", resp.StatusCode)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	feed := RSSFeed{}

	err = xml.Unmarshal(body, &feed)
	if err != nil {
		return nil, err
	}

	feed.Channel.Title = html.UnescapeString(feed.Channel.Title)
	feed.Channel.Description = html.UnescapeString(feed.Channel.Description)
	for i := range feed.Channel.Item {
		unescape(&feed.Channel.Item[i])
	}
	return &feed, nil
}

func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		errors.New("no duration given")
	}
	time_between_reqs := cmd.args[0]
	timeBetweenReqs, err := time.ParseDuration(time_between_reqs)
	if err != nil {
		return err
	}
	fmt.Printf("Collecting feeds every %s", time_between_reqs)

	ticker := time.NewTicker(timeBetweenReqs)
	for ; ; <-ticker.C {
		err = scrapeFeeds(s)
		if err != nil {
			return err
		}
	}
	return nil
}


func parseTime(dateStr string) (time.Time, error) {
    formats := []string{
        time.RFC1123Z,  // "Mon, 02 Jan 2006 15:04:05 -0700"
        time.RFC3339,   // "2006-01-02T15:04:05Z07:00"
		time.UnixDate,   // = "Mon Jan _2 15:04:05 MST 2006"
		time.RubyDate,   //= "Mon Jan 02 15:04:05 -0700 2006"
		time.RFC822,     // = "02 Jan 06 15:04 MST"
		time.RFC822Z,     //= "02 Jan 06 15:04 -0700" // RFC822 with numeric zone
		time.RFC850,     // = "Monday, 02-Jan-06 15:04:05 MST"
		time.RFC1123,   //  = "Mon, 02 Jan 2006 15:04:05 MST"
		time.RFC1123Z,    //= "Mon, 02 Jan 2006 15:04:05 -0700" // RFC1123 with numeric zone
		time.RFC3339,  //   = "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano, //= "2006-01-02T15:04:05.999999999Z07:00"
    }
    
    for _, format := range formats {
        if t, err := time.Parse(format, dateStr); err == nil {
            return t, nil
        }
    }
    
    return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

func scrapeFeeds(s *state) error {
	feed, err := s.db.GetNextFeedToFetch(context.Background())
	if err != nil {
		return err
	}
	sqlTime := sql.NullTime{time.Now(), true}
	args := database.MarkFeedFetchedParams{sqlTime, feed.ID}
	feed, err = s.db.MarkFeedFetched(context.Background(), args)
	if err != nil {
		return err
	}
	rssFeed, err := fetchFeed(context.Background(), feed.Url)
	if err != nil {
		return err
	}

	// fmt.Println(rssFeed.Channel.Title)
	for i := range rssFeed.Channel.Item {
		parsedTime, err := parseTime(rssFeed.Channel.Item[i].PubDate)
		if err != nil {
			return err
		}
		var sqlDescr sql.NullString
    
		if rssFeed.Channel.Item[i].Description != "" {
			sqlDescr = sql.NullString{String: rssFeed.Channel.Item[i].Description, Valid: true}
		} else {
			sqlDescr = sql.NullString{String: "", Valid: false}
		}
		
		args := database.CreatePostParams{
			uuid.New(),
			time.Now(),
			time.Now(),
			rssFeed.Channel.Item[i].Title,
			rssFeed.Channel.Item[i].Link,
			sqlDescr,
			parsedTime,
			feed.ID,
		}
		_, err = s.db.CreatePost(context.Background(), args)
		if err != nil {
			if strings.Contains(err.Error(), "unique_violation") ||
			   strings.Contains(err.Error(), "23505") ||
			   strings.Contains(err.Error(), "duplicate key") {
				continue
			} else{
				log.Printf("Error creating post: %v", err)
				return err
			}
		}
	}
	return nil
}


func handlerReset(s *state, cmd command) error {
	err := (s.db).Reset(context.Background())
	if err !=nil {
		return err
	}
	fmt.Println("Database reset")
	return nil
}

func handlerUsers(s *state, cmd command) error {
	users, err := (s.db).GetUsers(context.Background())
	if err !=nil {
		return err
	}
	for i:=0; i< len(users); i++ {
		if users[i] == (*s.configPtr).Current_user_name {
			fmt.Println("-", users[i], "(current)'")
		} else {
			fmt.Println("-", users[i])
		}
	}
	return nil
}

func handlerFeeds(s *state, cmd command) error {
	feeds, err := (s.db).GetFeeds(context.Background())
	if err != nil {
		return err
	}
	for i := range feeds {
		fmt.Println(feeds[i].Name)
		fmt.Println(feeds[i].Url)
		fmt.Println(feeds[i].CreatedBy)
	}
	return nil
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return errors.New("no user given")
	}

	_, err := (s.db).GetUser(context.Background(), cmd.args[0])
	if err !=nil {
		return err
	}

	err = (s.configPtr).SetUser(cmd.args[0])
	if err != nil {
		return err
	}
	fmt.Println("New user set")
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return errors.New("no user given")
	}

	args := database.CreateUserParams{uuid.New(), time.Now(), time.Now(), cmd.args[0]}
	_, err := (s.db).CreateUser(context.Background(), args)
	if err != nil {
		return err
	}

	err = (s.configPtr).SetUser(cmd.args[0])
	if err != nil {
		return err
	}
	fmt.Println("New user registered")
	fmt.Println("user name", cmd.args[0])
	return nil

}

func middlewareLoggedIn(handler func(s *state, cmd command, user database.User) error) func(*state, command) error {
	
	return func(s *state, cmd command) error {
		user_internal, err := s.db.GetUser(context.Background(), s.configPtr.Current_user_name)
		if err != nil {
			return err
		}
		return handler(s, cmd, user_internal)
	}
}

func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) == 0 {
		return errors.New("no url given")
	}

	ctx := context.Background()
	feed, err := (s.db).GetFeed(ctx, cmd.args[0])
	if err != nil {
		return err
	}

	
	args := database.CreateFeedFollowParams{uuid.New(), time.Now(), time.Now(), user.ID, feed.ID}
	feedFollow, err := s.db.CreateFeedFollow(ctx, args)
	if err != nil {
		return err
	}
	fmt.Println(feedFollow.FeedName)
	fmt.Println(feedFollow.UserName)
	return nil
}

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 2 {
		return errors.New("no name or url")
	}
	ctx := context.Background()
	args := database.CreateFeedParams{uuid.New(), time.Now(), time.Now(), cmd.args[0], cmd.args[1], user.ID}
	feed, err := s.db.CreateFeed(ctx, args)
	if err != nil {
		return err
	}

	argsFF := database.CreateFeedFollowParams{uuid.New(), time.Now(), time.Now(), user.ID, feed.ID}
	_, err = s.db.CreateFeedFollow(ctx, argsFF)
	if err != nil {
		return err
	}

	fmt.Println(feed.ID)
	fmt.Println(feed.CreatedAt)
	fmt.Println(feed.UpdatedAt)
	fmt.Println(feed.Name)
	fmt.Println(feed.Url)
	fmt.Println(feed.UserID)
	return nil
}

func handlerFollowing(s *state, cmd command, user database.User) error {
	feedFollows, err := s.db.GetFeedFollowsForUser(context.Background())
	if err != nil {
		return err
	}
	for i := range feedFollows {
		if feedFollows[i].UserName == user.Name{
			fmt.Println(feedFollows[i].FeedName)
		}
		
	}
	return nil
}

func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) == 0 {
		return errors.New("no url given")
	}
	args := database.UnfollowParams{user.Name, cmd.args[0]}
	_, err := s.db.Unfollow(context.Background(), args)
	if err != nil {
		return err
	}
	return nil
}

func handlerBrowse(s *state, cmd command) error {
	var limit int
	if len(cmd.args) == 0 {
		limit = 2
	} else{
		s := cmd.args[0]
		i, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		limit = i
	}
	user, err := (s.db).GetUser(context.Background(), s.configPtr.Current_user_name)
	if err !=nil {
		return err
	}
	args := database.GetPostsForUsersParams{user.ID, int32(limit)}
	posts, err := s.db.GetPostsForUsers(context.Background(), args)
	if err != nil {
		return err
	}
	for i := range(posts) {
		fmt.Println(posts[i].Title)
		if posts[i].Description.Valid {
			fmt.Println(posts[i].Description.String)
		}
	}
	return nil
}
type commands struct {
	handlers map[string]func(*state, command) error
}

func (c *commands) register(name string, f func(*state, command) error) {
	c.handlers[name] = f
}

func (c *commands) run(s *state, cmd command) error {
	f, ok := c.handlers[cmd.name]
	if ok == false {
		return errors.New("no such command")
	}
	
	return f(s, cmd)
}

func newCommands() *commands {
	return &commands{
		handlers: make(map[string]func(*state, command) error),
	}
}

func main() {
	con, err := config.Read()
	if err != nil {
		log.Fatal(err)
		return
	}

	dbURL := con.DBUrl
	conPtr := &con
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println("cannot open db", err)
		os.Exit(1)
	}
	dbQueries := database.New(db)

	s := state{dbQueries, conPtr}

	

	cmds := newCommands()
	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	cmds.register("feeds", handlerFeeds)
	cmds.register("follow", middlewareLoggedIn(handlerFollow))
	cmds.register("following", middlewareLoggedIn(handlerFollowing))
	cmds.register("unfollow", middlewareLoggedIn(handlerUnfollow))
	cmds.register("browse", handlerBrowse)
	args := os.Args
	if len(args) < 2 {
		log.Fatal("not enough arguments")
		return
	}
	cmdName := args[1]
	cmdArgs := args[2:]
	cmd := command{cmdName, cmdArgs}


	err = cmds.run(&s, cmd)
	if err != nil {
		log.Fatal(err)
		return
	}
}