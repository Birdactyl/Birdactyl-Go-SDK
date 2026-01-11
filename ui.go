package birdactyl

import (
	"embed"

	pb "github.com/Birdactyl/Birdactyl-Go-SDK/proto"
)

type UIBuilder struct {
	hasBundle    bool
	bundleData   []byte
	pages        []*pb.PluginUIPage
	tabs         []*pb.PluginUITab
	sidebarItems []*pb.PluginUISidebarItem
}

func newUIBuilder() *UIBuilder {
	return &UIBuilder{
		pages:        make([]*pb.PluginUIPage, 0),
		tabs:         make([]*pb.PluginUITab, 0),
		sidebarItems: make([]*pb.PluginUISidebarItem, 0),
	}
}

func (u *UIBuilder) HasBundle() *UIBuilder {
	u.hasBundle = true
	return u
}

func (u *UIBuilder) EmbedBundle(fs embed.FS, path string) *UIBuilder {
	data, err := fs.ReadFile(path)
	if err == nil {
		u.bundleData = data
		u.hasBundle = true
	}
	return u
}

func (u *UIBuilder) BundleBytes(data []byte) *UIBuilder {
	u.bundleData = data
	u.hasBundle = true
	return u
}

func (u *UIBuilder) GetBundleData() []byte {
	return u.bundleData
}

func (u *UIBuilder) Page(path, component string) *UIPageBuilder {
	page := &pb.PluginUIPage{
		Path:      path,
		Component: component,
	}
	u.pages = append(u.pages, page)
	return &UIPageBuilder{page: page, ui: u}
}

func (u *UIBuilder) Tab(id, component, target, label string) *UITabBuilder {
	tab := &pb.PluginUITab{
		Id:        id,
		Component: component,
		Target:    target,
		Label:     label,
	}
	u.tabs = append(u.tabs, tab)
	return &UITabBuilder{tab: tab, ui: u}
}

func (u *UIBuilder) SidebarItem(id, label, href, section string) *UISidebarBuilder {
	item := &pb.PluginUISidebarItem{
		Id:      id,
		Label:   label,
		Href:    href,
		Section: section,
	}
	u.sidebarItems = append(u.sidebarItems, item)
	return &UISidebarBuilder{item: item, ui: u}
}

func (u *UIBuilder) build() *pb.PluginUIInfo {
	return &pb.PluginUIInfo{
		HasBundle:    u.hasBundle,
		Pages:        u.pages,
		Tabs:         u.tabs,
		SidebarItems: u.sidebarItems,
		BundleData:   u.bundleData,
	}
}

type UIPageBuilder struct {
	page *pb.PluginUIPage
	ui   *UIBuilder
}

func (p *UIPageBuilder) Title(title string) *UIPageBuilder {
	p.page.Title = title
	return p
}

func (p *UIPageBuilder) Icon(icon string) *UIPageBuilder {
	p.page.Icon = icon
	return p
}

func (p *UIPageBuilder) AdminOnly() *UIPageBuilder {
	p.page.Guard = "admin"
	return p
}

func (p *UIPageBuilder) Guard(guard string) *UIPageBuilder {
	p.page.Guard = guard
	return p
}

func (p *UIPageBuilder) Done() *UIBuilder {
	return p.ui
}

type UITabBuilder struct {
	tab *pb.PluginUITab
	ui  *UIBuilder
}

func (t *UITabBuilder) Icon(icon string) *UITabBuilder {
	t.tab.Icon = icon
	return t
}

func (t *UITabBuilder) Order(order int) *UITabBuilder {
	t.tab.Order = int32(order)
	return t
}

func (t *UITabBuilder) Done() *UIBuilder {
	return t.ui
}

type UISidebarBuilder struct {
	item *pb.PluginUISidebarItem
	ui   *UIBuilder
}

func (s *UISidebarBuilder) Icon(icon string) *UISidebarBuilder {
	s.item.Icon = icon
	return s
}

func (s *UISidebarBuilder) Order(order int) *UISidebarBuilder {
	s.item.Order = int32(order)
	return s
}

func (s *UISidebarBuilder) AdminOnly() *UISidebarBuilder {
	s.item.Guard = "admin"
	return s
}

func (s *UISidebarBuilder) Child(label, href string) *UISidebarBuilder {
	s.item.Children = append(s.item.Children, &pb.PluginUISidebarChild{
		Label: label,
		Href:  href,
	})
	return s
}

func (s *UISidebarBuilder) Done() *UIBuilder {
	return s.ui
}

const (
	TabTargetServer       = "server"
	TabTargetUserSettings = "user-settings"

	SidebarSectionNav      = "nav"
	SidebarSectionPlatform = "platform"
	SidebarSectionAdmin    = "admin"
)
