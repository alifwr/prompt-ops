import { pgTable, serial, text, timestamp, integer } from 'drizzle-orm/pg-core';

export const users = pgTable('users', {
  id: serial('id').primaryKey(),
  email: text('email').notNull().unique(),
  passwordHash: text('password_hash').notNull(),
  createdAt: timestamp('created_at').defaultNow().notNull(),
});

export const servers = pgTable('servers', {
  id: serial('id').primaryKey(),
  name: text('name').notNull(),
  ipAddress: text('ip_address').notNull(),
  status: text('status').default('offline').notNull(), // online, offline, registering
  token: text('token').notNull().unique(),
  createdAt: timestamp('created_at').defaultNow().notNull(),
});

export const deployments = pgTable('deployments', {
  id: serial('id').primaryKey(),
  serverId: integer('server_id').references(() => servers.id).notNull(),
  projectName: text('project_name').notNull(),
  composeYaml: text('compose_yaml').notNull(),
  status: text('status').default('running').notNull(), // running, stopped, failed
  createdAt: timestamp('created_at').defaultNow().notNull(),
});

export const auditLogs = pgTable('audit_logs', {
  id: serial('id').primaryKey(),
  userId: integer('user_id').references(() => users.id),
  serverId: integer('server_id').references(() => servers.id),
  action: text('action').notNull(), // "deploy", "stop_container", "start_tty"
  details: text('details').notNull(), // JSON logs or command strings
  createdAt: timestamp('created_at').defaultNow().notNull(),
});
