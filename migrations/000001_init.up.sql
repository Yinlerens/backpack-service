create schema if not exists backpack;

create table if not exists backpack.pull_events (
  event_id uuid primary key,
  user_id uuid not null,
  event_type text not null,
  banner_id text not null,
  seed text not null,
  state_version bigint not null,
  previous_pity jsonb not null,
  next_pity jsonb not null,
  raw_event jsonb not null,
  received_at timestamptz not null default now(),
  constraint pull_events_event_type_length check (length(event_type) > 0 and length(event_type) <= 100),
  constraint pull_events_banner_id_length check (length(banner_id) > 0 and length(banner_id) <= 100),
  constraint pull_events_seed_length check (length(seed) > 0 and length(seed) <= 200),
  constraint pull_events_state_version_non_negative check (state_version >= 0),
  constraint pull_events_previous_pity_object check (jsonb_typeof(previous_pity) = 'object'),
  constraint pull_events_next_pity_object check (jsonb_typeof(next_pity) = 'object'),
  constraint pull_events_raw_event_object check (jsonb_typeof(raw_event) = 'object')
);

create table if not exists backpack.pull_records (
  id uuid primary key,
  event_id uuid not null references backpack.pull_events(event_id) on delete restrict,
  user_id uuid not null,
  record_index integer not null,
  item_id text not null,
  item_name text not null,
  item_type text not null,
  rarity integer not null,
  banner_id text not null,
  banner_name text not null,
  pity_at_five integer not null,
  pity_at_four integer not null,
  is_featured boolean not null,
  received_at timestamptz not null default now(),
  constraint pull_records_record_index_non_negative check (record_index >= 0),
  constraint pull_records_item_id_length check (length(item_id) > 0 and length(item_id) <= 100),
  constraint pull_records_item_name_length check (length(item_name) > 0 and length(item_name) <= 200),
  constraint pull_records_item_type_check check (item_type in ('character', 'weapon')),
  constraint pull_records_rarity_check check (rarity in (3, 4, 5)),
  constraint pull_records_banner_id_length check (length(banner_id) > 0 and length(banner_id) <= 100),
  constraint pull_records_banner_name_length check (length(banner_name) > 0 and length(banner_name) <= 200),
  constraint pull_records_pity_at_five_positive check (pity_at_five >= 1),
  constraint pull_records_pity_at_four_positive check (pity_at_four >= 1),
  constraint pull_records_event_index_unique unique (event_id, record_index)
);

create table if not exists backpack.inventory_items (
  user_id uuid not null,
  item_id text not null,
  item_name text not null,
  item_type text not null,
  rarity integer not null,
  quantity bigint not null,
  first_received_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (user_id, item_id),
  constraint inventory_items_item_id_length check (length(item_id) > 0 and length(item_id) <= 100),
  constraint inventory_items_item_name_length check (length(item_name) > 0 and length(item_name) <= 200),
  constraint inventory_items_item_type_check check (item_type in ('character', 'weapon')),
  constraint inventory_items_rarity_check check (rarity in (3, 4, 5)),
  constraint inventory_items_quantity_positive check (quantity > 0)
);

create index if not exists pull_events_user_received_idx
  on backpack.pull_events (user_id, received_at desc, event_id desc);

create index if not exists pull_events_user_banner_received_idx
  on backpack.pull_events (user_id, banner_id, received_at desc, event_id desc);

create index if not exists pull_records_user_received_idx
  on backpack.pull_records (user_id, received_at desc, id desc);

create index if not exists pull_records_user_banner_received_idx
  on backpack.pull_records (user_id, banner_id, received_at desc, id desc);

create index if not exists pull_records_user_rarity_received_idx
  on backpack.pull_records (user_id, rarity, received_at desc, id desc);

create index if not exists inventory_items_user_updated_idx
  on backpack.inventory_items (user_id, updated_at desc, item_id desc);

