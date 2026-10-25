<script setup lang="ts">
import { svgDataUri } from "@/utils/svg-data-uri";
import { ref, computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { NAVIGATION_CONFIG, type NavItem } from "@/config/navigation";
import { usePlatformConfigStore } from "@/stores/platformConfig";
import { useAppThemeStore } from "@/stores/appTheme";
import { useAuthStore } from "@/stores/auth";
import { canAccessPath, canSeeScope, userHasPermission } from "@/stores/permissions";
import {
	isSectionOpen,
	loadClosedSections,
	saveClosedSections,
	sectionHasActive,
	toggleSection,
} from "@/utils/nav-sections";

const props = defineProps<{
	collapsed: boolean;
	mobileOpen?: boolean;
}>();

const emit = defineEmits<{
	toggleCollapse: [];
}>();

const route = useRoute();
const platformConfigStore = usePlatformConfigStore();
const appThemeStore = useAppThemeStore();
const authStore = useAuthStore();
const expandedItems = ref<Record<string, boolean>>({});

// Collapsed section labels, persisted per viewer. The section holding the
// current page is always shown open (see utils/nav-sections), so navigating
// into a collapsed section reveals it without touching the stored choice.
const closedSections = ref<Set<string>>(loadClosedSections());

// The icon-only sidebar hides the section headers, so it shows every entry.
function sectionOpen(group: { label: string; items: NavItem[] }): boolean {
	return props.collapsed || isSectionOpen(group, closedSections.value, route.path);
}

function onToggleSection(group: { label: string; items: NavItem[] }) {
	closedSections.value = toggleSection(group, closedSections.value, route.path);
	saveClosedSections(closedSections.value);
}

function sectionId(label: string): string {
	return "nav-sec-" + label.toLowerCase().replace(/[^a-z0-9]+/g, "-");
}

// Keep only the nav entries the current user can actually reach: filter a
// parent's children (dropping the parent if none remain) and leaf items by the
// same canAccessPath() the route guards use, so the sidebar never offers a page
// that would bounce the user to their profile.
function visibleItem(item: NavItem): NavItem | null {
	if (!canSeeScope(authStore.user, item.scope)) return null;
	if (item.children && item.children.length > 0) {
		const children = item.children.filter(
			(c) =>
				canSeeScope(authStore.user, c.scope) &&
				(!c.route || canAccessPath(authStore.user, c.route)),
		);
		return children.length > 0 ? { ...item, children } : null;
	}
	if (item.feature === "ingestion" && !platformConfigStore.ingestionEnabled) {
		return null;
	}
	if (
		item.href &&
		item.permission &&
		!userHasPermission(authStore.user, item.permission)
	) {
		return null;
	}
	if (item.route && !canAccessPath(authStore.user, item.route)) return null;
	return item;
}

// Load app theme on mount
onMounted(() => {
	appThemeStore.loadTheme();
});

// Filter navigation by platform configuration and per-user access.
const filteredNavigation = computed(() => {
	return NAVIGATION_CONFIG.filter((group) => {
		// Hide Messaging group when messaging is disabled
		if (group.label === "Messaging" && !platformConfigStore.messagingEnabled) {
			return false;
		}
		return true;
	})
		.map((group) => ({
			...group,
			items: group.items
				.map(visibleItem)
				.filter((item): item is NavItem => item !== null),
		}))
		// Drop groups that have no visible items for this user.
		.filter((group) => group.items.length > 0);
});

function toggleExpand(itemLabel: string) {
	expandedItems.value = {
		...expandedItems.value,
		[itemLabel]: !expandedItems.value[itemLabel],
	};
}

function isActive(item: NavItem): boolean {
	if (!item.route) return false;
	return route.path === item.route || route.path.startsWith(item.route + "/");
}
</script>

<template>
  <aside class="sidebar" :class="{ collapsed, 'mobile-open': mobileOpen }">
    <!-- Logo Section -->
    <div class="sidebar-header">
      <div
        v-if="!collapsed"
        class="logo-container"
        :style="{ height: `${appThemeStore.logoHeight}px` }"
      >
        <!-- Custom logo URL -->
        <img
          v-if="appThemeStore.logoUrl"
          :src="appThemeStore.logoUrl"
          class="logo-image"
          :style="{ maxHeight: `${appThemeStore.logoHeight}px` }"
          alt="Logo"
        />
        <!-- Custom logo SVG -->
        <!-- An image, never v-html: an SVG in <img> runs no script. -->
        <img
          v-else-if="appThemeStore.logoSvg"
          :src="svgDataUri(appThemeStore.logoSvg)"
          class="logo-image"
          :style="{ height: `${appThemeStore.logoHeight}px` }"
          alt="Logo"
        />
        <!-- Default logo icon -->
        <div
          v-else
          class="logo-icon"
          :style="{
            width: `${appThemeStore.logoHeight}px`,
            height: `${appThemeStore.logoHeight}px`,
          }"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M13 10V3L4 14h7v7l9-11h-7z"
            />
          </svg>
        </div>
      </div>
      <button class="collapse-btn" @click="emit('toggleCollapse')">
        <i class="pi" :class="collapsed ? 'pi-chevron-right' : 'pi-chevron-left'"></i>
      </button>
    </div>

    <!-- Navigation -->
    <nav class="sidebar-nav">
      <div v-for="group in filteredNavigation" :key="group.label" class="nav-group">
        <button
          v-if="!collapsed"
          type="button"
          class="nav-group-label"
          :aria-expanded="sectionOpen(group)"
          :aria-controls="sectionId(group.label)"
          :aria-disabled="sectionHasActive(group, route.path) || undefined"
          :title="sectionHasActive(group, route.path) ? 'Contains the current page' : undefined"
          @click="onToggleSection(group)"
        >
          <span>{{ group.label }}</span>
          <i
            class="pi section-chevron"
            :class="sectionOpen(group) ? 'pi-chevron-down' : 'pi-chevron-right'"
            aria-hidden="true"
          ></i>
        </button>

        <div v-show="sectionOpen(group)" :id="sectionId(group.label)">
        <template v-for="item in group.items" :key="item.label">
          <!-- Parent with children -->
          <div v-if="item.children && item.children.length > 0" class="nav-item-wrapper">
            <button
              class="nav-item has-children"
              :class="{ expanded: expandedItems[item.label] }"
              :title="collapsed ? item.label : ''"
              @click="toggleExpand(item.label)"
            >
              <i :class="item.icon"></i>
              <template v-if="!collapsed">
                <span class="nav-label">{{ item.label }}</span>
                <i
                  class="pi expand-icon"
                  :class="expandedItems[item.label] ? 'pi-chevron-down' : 'pi-chevron-right'"
                ></i>
              </template>
            </button>
            <div v-if="!collapsed && expandedItems[item.label]" class="nav-children">
              <RouterLink
                v-for="child in item.children"
                :key="child.label"
                :to="child.route || '#'"
                class="nav-child-item"
                :class="{ active: isActive(child) }"
              >
                <i :class="child.icon"></i>
                <span class="nav-label">{{ child.label }}</span>
              </RouterLink>
            </div>
          </div>

          <!-- Page outside the SPA (server-rendered /ui) -->
          <a
            v-else-if="item.href"
            :href="item.href"
            class="nav-item"
            :title="collapsed ? item.label : ''"
          >
            <i :class="item.icon"></i>
            <span v-if="!collapsed" class="nav-label">{{ item.label }}</span>
          </a>

          <!-- Simple nav item -->
          <RouterLink
            v-else
            :to="item.route || '#'"
            class="nav-item"
            :class="{ active: isActive(item) }"
            :title="collapsed ? item.label : ''"
          >
            <i :class="item.icon"></i>
            <span v-if="!collapsed" class="nav-label">{{ item.label }}</span>
          </RouterLink>
        </template>
        </div>
      </div>
    </nav>

    <!-- Footer -->
    <div class="sidebar-footer">
      <SidebarProfile :collapsed="collapsed" />
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  position: fixed;
  left: 0;
  top: 0;
  bottom: 0;
  width: 260px;
  background: linear-gradient(180deg, #102a43 0%, #0a1929 100%);
  display: flex;
  flex-direction: column;
  transition: width 0.3s ease;
  z-index: 1000;
  overflow: hidden;
}

.sidebar.collapsed {
  width: 72px;
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  min-height: 64px;
}

.logo-container {
  display: flex;
  align-items: center;
}

.logo-icon {
  color: #47a3f3;
  flex-shrink: 0;
}

.logo-icon svg {
  width: 100%;
  height: 100%;
}

.logo-image {
  width: auto;
  object-fit: contain;
  flex-shrink: 0;
}

.collapse-btn {
  background: rgba(255, 255, 255, 0.1);
  border: none;
  border-radius: 6px;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: rgba(255, 255, 255, 0.7);
  transition: all 0.2s ease;
  flex-shrink: 0;
}

.collapse-btn:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #ffffff;
}

.sidebar.collapsed .collapse-btn {
  margin: 0 auto;
}

.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 16px 0;
}

.nav-group {
  margin-bottom: 24px;
}

.nav-group-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  background: none;
  border: none;
  text-align: left;
  cursor: pointer;
  padding: 0 20px;
  margin-bottom: 8px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: rgba(255, 255, 255, 0.4);
}

.nav-group-label:hover {
  color: rgba(255, 255, 255, 0.7);
}

.nav-group-label:focus-visible {
  outline: 2px solid #47a3f3;
  outline-offset: -2px;
}

.nav-group-label[aria-disabled="true"] {
  cursor: default;
}

.section-chevron {
  font-size: 10px;
}

.nav-item-wrapper {
  display: flex;
  flex-direction: column;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 20px;
  color: rgba(255, 255, 255, 0.7);
  text-decoration: none;
  transition: all 0.2s ease;
  cursor: pointer;
  border: none;
  background: none;
  width: 100%;
  text-align: left;
  font-size: 14px;
}

.nav-item i {
  font-size: 18px;
  width: 24px;
  text-align: center;
  flex-shrink: 0;
}

.nav-item:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #ffffff;
}

.nav-item.active {
  background: rgba(71, 163, 243, 0.15);
  color: #47a3f3;
  border-right: 3px solid #47a3f3;
}

.nav-item.has-children {
  position: relative;
}

.nav-item .expand-icon {
  margin-left: auto;
  font-size: 12px;
  transition: transform 0.2s ease;
}

.nav-children {
  padding-left: 12px;
  overflow: hidden;
}

.nav-child-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 20px 8px 32px;
  color: rgba(255, 255, 255, 0.6);
  text-decoration: none;
  transition: all 0.2s ease;
  font-size: 13px;
}

.nav-child-item i {
  font-size: 14px;
  width: 20px;
  text-align: center;
}

.nav-child-item:hover {
  color: rgba(255, 255, 255, 0.9);
  background: rgba(255, 255, 255, 0.05);
}

.nav-child-item.active {
  color: #47a3f3;
  background: rgba(71, 163, 243, 0.1);
}

.nav-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar.collapsed .nav-item {
  justify-content: center;
  padding: 12px;
}

.sidebar.collapsed .nav-group-label {
  display: none;
}

.sidebar-footer {
  padding: 8px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
}

/* Scrollbar styling */
.sidebar-nav::-webkit-scrollbar {
  width: 4px;
}

.sidebar-nav::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-nav::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
}

.sidebar-nav::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}

@media (max-width: 768px) {
  .sidebar,
  .sidebar.collapsed {
    width: 260px;
    transform: translateX(-100%);
  }

  .sidebar.mobile-open {
    transform: translateX(0);
  }
}
</style>
