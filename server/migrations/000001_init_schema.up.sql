-- 000001_init_schema.up.sql

-- 1. runs: stores search/scrape runs
CREATE TABLE IF NOT EXISTS runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    query TEXT NOT NULL DEFAULT '',
    time_filter TEXT NOT NULL DEFAULT '',
    sort_by TEXT NOT NULL DEFAULT '',
    trigger TEXT NOT NULL DEFAULT 'manual' CHECK (trigger IN ('manual', 'auto')),
    status TEXT NOT NULL DEFAULT 'pending',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    stats JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at DESC);

-- 2. posts: found job/opportunity posts
CREATE TABLE IF NOT EXISTS posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url TEXT NOT NULL UNIQUE,
    author TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    company TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    work_mode TEXT NOT NULL DEFAULT '',
    paid_status TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    extracted JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'new',
    skip_reason TEXT,
    found_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_posts_status ON posts(status);
CREATE INDEX IF NOT EXISTS idx_posts_found_at ON posts(found_at DESC);

-- 3. contacts: extracted leads / contacts
CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    company TEXT NOT NULL DEFAULT '',
    domain TEXT NOT NULL DEFAULT '',
    trust_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    trust_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    post_id UUID REFERENCES posts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);
CREATE INDEX IF NOT EXISTS idx_contacts_post_id ON contacts(post_id);

-- 4. outreach: email outreach pipeline
CREATE TABLE IF NOT EXISTS outreach (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contact_id UUID REFERENCES contacts(id) ON DELETE CASCADE,
    post_id UUID REFERENCES posts(id) ON DELETE SET NULL,
    kind TEXT NOT NULL DEFAULT 'initial' CHECK (kind IN ('initial', 'followup1', 'followup2')),
    subject TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'queued', 'sent', 'failed', 'skipped', 'replied')),
    gmail_message_id TEXT,
    gmail_thread_id TEXT,
    scheduled_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outreach_status ON outreach(status);
CREATE INDEX IF NOT EXISTS idx_outreach_contact_id ON outreach(contact_id);
CREATE INDEX IF NOT EXISTS idx_outreach_post_id ON outreach(post_id);
CREATE INDEX IF NOT EXISTS idx_outreach_kind ON outreach(kind);

-- 5. replies: tracking replies received via Gmail
CREATE TABLE IF NOT EXISTS replies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outreach_id UUID REFERENCES outreach(id) ON DELETE CASCADE,
    gmail_message_id TEXT,
    snippet TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_replies_outreach_id ON replies(outreach_id);

-- 6. settings: key-value configuration store
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 7. profile: candidate profile and context
CREATE TABLE IF NOT EXISTS profile (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    summary TEXT NOT NULL DEFAULT '',
    skills TEXT NOT NULL DEFAULT '',
    resume_path TEXT NOT NULL DEFAULT '',
    resume_text TEXT NOT NULL DEFAULT '',
    example_text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
