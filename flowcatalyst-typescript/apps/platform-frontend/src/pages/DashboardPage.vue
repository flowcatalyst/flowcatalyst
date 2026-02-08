<script setup lang="ts">
import { useAuthStore } from '@/stores/auth';

const authStore = useAuthStore();

const dashboardCards = [
  {
    title: 'Applications',
    description: 'Manage applications in the platform ecosystem',
    route: '/applications',
    icon: 'pi pi-th-large',
    bgColor: 'bg-indigo',
    iconColor: 'text-indigo',
  },
  {
    title: 'Clients',
    description: 'Manage clients and their configurations',
    route: '/clients',
    icon: 'pi pi-building',
    bgColor: 'bg-blue',
    iconColor: 'text-blue',
  },
  {
    title: 'Users',
    description: 'Manage platform users and their access',
    route: '/users',
    icon: 'pi pi-users',
    bgColor: 'bg-green',
    iconColor: 'text-green',
  },
  {
    title: 'Roles',
    description: 'Configure roles and permissions',
    route: '/roles',
    icon: 'pi pi-shield',
    bgColor: 'bg-purple',
    iconColor: 'text-purple',
  },
  {
    title: 'Event Types',
    description: 'Define event types and schemas for messaging',
    route: '/event-types',
    icon: 'pi pi-bolt',
    bgColor: 'bg-amber',
    iconColor: 'text-amber',
  },
  {
    title: 'Subscriptions',
    description: 'Manage event subscriptions and routing',
    route: '/subscriptions',
    icon: 'pi pi-bell',
    bgColor: 'bg-teal',
    iconColor: 'text-teal',
  },
];
</script>

<template>
  <div class="dashboard-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">Dashboard</h1>
        <p class="page-subtitle">Welcome back, {{ authStore.displayName }}</p>
      </div>
    </div>

    <!-- Quick actions grid -->
    <div class="cards-grid">
      <RouterLink
        v-for="card in dashboardCards"
        :key="card.title"
        :to="card.route"
        class="dashboard-card"
      >
        <div class="card-content">
          <div class="card-icon" :class="card.bgColor">
            <i :class="[card.icon, card.iconColor]"></i>
          </div>
          <div class="card-info">
            <h3 class="card-title">{{ card.title }}</h3>
            <p class="card-description">{{ card.description }}</p>
          </div>
        </div>
      </RouterLink>
    </div>

    <!-- Stats section -->
    <div class="stats-section">
      <h2 class="section-title">Platform Overview</h2>
      <div class="stats-grid">
        <div class="stat-card">
          <p class="stat-label">Total Clients</p>
          <p class="stat-value">--</p>
        </div>
        <div class="stat-card">
          <p class="stat-label">Active Users</p>
          <p class="stat-value">--</p>
        </div>
        <div class="stat-card">
          <p class="stat-label">Roles Defined</p>
          <p class="stat-value">--</p>
        </div>
        <div class="stat-card">
          <p class="stat-label">API Calls (24h)</p>
          <p class="stat-value">--</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard-page {
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: 32px;
}

.page-title {
  font-size: 28px;
  font-weight: 600;
  color: #102a43;
  margin: 0;
}

.page-subtitle {
  color: #627d98;
  margin: 8px 0 0;
  font-size: 15px;
}

.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 20px;
  margin-bottom: 48px;
}

.dashboard-card {
  background: white;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  padding: 20px;
  text-decoration: none;
  transition: all 0.2s ease;
}

.dashboard-card:hover {
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
  border-color: #cbd5e1;
  transform: translateY(-2px);
}

.card-content {
  display: flex;
  gap: 16px;
}

.card-icon {
  width: 48px;
  height: 48px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.card-icon i {
  font-size: 20px;
}

.card-icon.bg-indigo {
  background: #e0e7ff;
}
.card-icon.bg-indigo .text-indigo {
  color: #4f46e5;
}

.card-icon.bg-blue {
  background: #dbeafe;
}
.card-icon.bg-blue .text-blue {
  color: #2563eb;
}

.card-icon.bg-green {
  background: #dcfce7;
}
.card-icon.bg-green .text-green {
  color: #16a34a;
}

.card-icon.bg-purple {
  background: #f3e8ff;
}
.card-icon.bg-purple .text-purple {
  color: #9333ea;
}

.card-icon.bg-amber {
  background: #fef3c7;
}
.card-icon.bg-amber .text-amber {
  color: #d97706;
}

.card-icon.bg-teal {
  background: #ccfbf1;
}
.card-icon.bg-teal .text-teal {
  color: #0d9488;
}

.card-info {
  flex: 1;
  min-width: 0;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
  margin: 0 0 4px;
  transition: color 0.2s ease;
}

.dashboard-card:hover .card-title {
  color: #0967d2;
}

.card-description {
  font-size: 14px;
  color: #64748b;
  margin: 0;
  line-height: 1.4;
}

.stats-section {
  margin-top: 48px;
}

.section-title {
  font-size: 18px;
  font-weight: 600;
  color: #243b53;
  margin: 0 0 16px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.stat-card {
  background: white;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  padding: 20px;
}

.stat-label {
  font-size: 14px;
  color: #64748b;
  margin: 0 0 8px;
}

.stat-value {
  font-size: 28px;
  font-weight: 600;
  color: #102a43;
  margin: 0;
}
</style>
