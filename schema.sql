
create table links(linkid integer primary key, textid integer, url text, dt text, source text, site text);
create virtual table linktext using fts4 (title, summary, remnants);
create table tags (tagid integer primary key, linkid integer, tag text);
create table sources (sourceid integer primary key, name text, notes text);

create table followers(followerid integer primary key, url text);

create index idx_linkstextid on links(textid);
create index idx_linkssite on links(site);
create index idx_linkssource on links(source);
create index idx_tagstag on tags(tag);
create index idx_tagslinkid on tags(linkid);

CREATE TABLE config (key text, value text);

CREATE TABLE users (userid integer primary key, username text, hash text);
CREATE TABLE auth (authid integer primary key, userid integer, hash text, expiry text);
CREATE INDEX idxusers_username on users(username);
CREATE INDEX idxauth_userid on auth(userid);
CREATE INDEX idxauth_hash on auth(hash);

