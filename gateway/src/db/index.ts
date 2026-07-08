import { drizzle } from 'drizzle-orm/node-postgres';
import { Pool } from 'pg';
import * as schema from './schema';

const databaseUrl = process.env.DATABASE_URL || 'postgresql://postgres:postgres@localhost:5432/promptops';

export const pool = new Pool({
  connectionString: databaseUrl,
});

pool.on('error', (err) => {
  console.warn('[DB WARNING] PostgreSQL Pool Error: ' + err.message);
});

export const db = drizzle(pool, { schema });
