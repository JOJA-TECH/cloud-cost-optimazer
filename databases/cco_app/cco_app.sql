--
-- PostgreSQL database dump
--

\restrict Sj7lMTD4WhhaYCnLeLJJzzl3YyCE0AtJtpAodaeoM88rBhibvHxf2Rb5MQBS9OG

-- Dumped from database version 18.6
-- Dumped by pg_dump version 18.6

-- Started on 2026-09-27 21:11:33

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- TOC entry 11 (class 2615 OID 17276)
-- Name: app; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA app;


ALTER SCHEMA app OWNER TO postgres;

--
-- TOC entry 12 (class 2615 OID 17277)
-- Name: audit; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA audit;


ALTER SCHEMA audit OWNER TO postgres;

--
-- TOC entry 10 (class 2615 OID 17275)
-- Name: cloud; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA cloud;


ALTER SCHEMA cloud OWNER TO postgres;

--
-- TOC entry 8 (class 2615 OID 17273)
-- Name: iam; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA iam;


ALTER SCHEMA iam OWNER TO postgres;

--
-- TOC entry 9 (class 2615 OID 17274)
-- Name: organization; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA organization;


ALTER SCHEMA organization OWNER TO postgres;

--
-- TOC entry 3 (class 3079 OID 17168)
-- Name: citext; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;


--
-- TOC entry 5391 (class 0 OID 0)
-- Dependencies: 3
-- Name: EXTENSION citext; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION citext IS 'data type for case-insensitive character strings';


--
-- TOC entry 2 (class 3079 OID 17130)
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- TOC entry 5392 (class 0 OID 0)
-- Dependencies: 2
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- TOC entry 309 (class 1255 OID 17847)
-- Name: current_user_id(); Type: FUNCTION; Schema: app; Owner: postgres
--

CREATE FUNCTION app.current_user_id() RETURNS uuid
    LANGUAGE sql STABLE
    AS $$
    SELECT NULLIF(current_setting('app.user_id', true), '')::uuid;
$$;


ALTER FUNCTION app.current_user_id() OWNER TO postgres;

--
-- TOC entry 347 (class 1255 OID 17849)
-- Name: is_org_member(uuid); Type: FUNCTION; Schema: app; Owner: postgres
--

CREATE FUNCTION app.is_org_member(p_organization_id uuid) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'iam', 'organization', 'app'
    AS $$
    SELECT EXISTS (
        SELECT 1
        FROM organization.members m
        WHERE m.organization_id = p_organization_id
          AND m.user_id = app.current_user_id()
          AND m.status = 'active'
    );
$$;


ALTER FUNCTION app.is_org_member(p_organization_id uuid) OWNER TO postgres;

--
-- TOC entry 338 (class 1255 OID 17848)
-- Name: is_platform_admin(); Type: FUNCTION; Schema: app; Owner: postgres
--

CREATE FUNCTION app.is_platform_admin() RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT EXISTS (
        SELECT 1
        FROM iam.user_roles ur
        JOIN iam.roles r ON r.role_id = ur.role_id
        WHERE ur.user_id = app.current_user_id()
          AND r.role_code = 'platform_admin'
    );
$$;


ALTER FUNCTION app.is_platform_admin() OWNER TO postgres;

--
-- TOC entry 316 (class 1255 OID 17850)
-- Name: is_project_member(uuid); Type: FUNCTION; Schema: app; Owner: postgres
--

CREATE FUNCTION app.is_project_member(p_project_id uuid) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'iam', 'organization', 'app'
    AS $$
    SELECT EXISTS (
        SELECT 1
        FROM organization.projects p
        JOIN organization.members m ON m.organization_id = p.organization_id
        WHERE p.project_id = p_project_id
          AND m.user_id = app.current_user_id()
          AND m.status = 'active'
    );
$$;


ALTER FUNCTION app.is_project_member(p_project_id uuid) OWNER TO postgres;

--
-- TOC entry 321 (class 1255 OID 17838)
-- Name: set_updated_at(); Type: FUNCTION; Schema: app; Owner: postgres
--

CREATE FUNCTION app.set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;


ALTER FUNCTION app.set_updated_at() OWNER TO postgres;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- TOC entry 241 (class 1259 OID 17596)
-- Name: dashboard_preferences; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.dashboard_preferences (
    dashboard_preference_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    project_id uuid,
    layout jsonb DEFAULT '{}'::jsonb NOT NULL,
    widgets jsonb DEFAULT '[]'::jsonb NOT NULL,
    default_period character varying(20) DEFAULT '30d'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_dashboard_layout_object CHECK ((jsonb_typeof(layout) = 'object'::text)),
    CONSTRAINT ck_dashboard_period CHECK (((default_period)::text = ANY ((ARRAY['24h'::character varying, '7d'::character varying, '30d'::character varying, '90d'::character varying, '1y'::character varying])::text[]))),
    CONSTRAINT ck_dashboard_widgets_array CHECK ((jsonb_typeof(widgets) = 'array'::text))
);


ALTER TABLE app.dashboard_preferences OWNER TO postgres;

--
-- TOC entry 243 (class 1259 OID 17664)
-- Name: notification_deliveries; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.notification_deliveries (
    notification_delivery_id uuid DEFAULT gen_random_uuid() NOT NULL,
    notification_id uuid NOT NULL,
    channel character varying(20) NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    provider_message_id character varying(255),
    attempts smallint DEFAULT 0 NOT NULL,
    last_attempt_at timestamp with time zone,
    delivered_at timestamp with time zone,
    failure_reason text,
    CONSTRAINT ck_delivery_attempts CHECK ((attempts >= 0)),
    CONSTRAINT ck_delivery_channel CHECK (((channel)::text = ANY ((ARRAY['in_app'::character varying, 'email'::character varying, 'webhook'::character varying])::text[]))),
    CONSTRAINT ck_delivery_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'sent'::character varying, 'delivered'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))
);


ALTER TABLE app.notification_deliveries OWNER TO postgres;

--
-- TOC entry 242 (class 1259 OID 17633)
-- Name: notifications; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.notifications (
    notification_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    organization_id uuid,
    type character varying(60) NOT NULL,
    title character varying(200) NOT NULL,
    message text NOT NULL,
    severity character varying(20) DEFAULT 'info'::character varying NOT NULL,
    reference_type character varying(50),
    reference_id uuid,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone,
    CONSTRAINT ck_notifications_severity CHECK (((severity)::text = ANY ((ARRAY['info'::character varying, 'success'::character varying, 'warning'::character varying, 'critical'::character varying])::text[])))
);


ALTER TABLE app.notifications OWNER TO postgres;

--
-- TOC entry 247 (class 1259 OID 17779)
-- Name: recommendation_interactions; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.recommendation_interactions (
    recommendation_interaction_id uuid DEFAULT gen_random_uuid() CONSTRAINT recommendation_interactions_recommendation_interaction_not_null NOT NULL,
    user_id uuid NOT NULL,
    project_id uuid NOT NULL,
    recommendation_uid uuid NOT NULL,
    action character varying(30) NOT NULL,
    comment text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_recommendation_action CHECK (((action)::text = ANY ((ARRAY['viewed'::character varying, 'accepted'::character varying, 'rejected'::character varying, 'deferred'::character varying, 'dismissed'::character varying, 'restored'::character varying])::text[])))
);


ALTER TABLE app.recommendation_interactions OWNER TO postgres;

--
-- TOC entry 246 (class 1259 OID 17756)
-- Name: report_artifacts; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.report_artifacts (
    report_artifact_id uuid DEFAULT gen_random_uuid() NOT NULL,
    report_request_id uuid NOT NULL,
    storage_uri text NOT NULL,
    mime_type character varying(100) NOT NULL,
    file_size_bytes bigint,
    checksum_sha256 character(64),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone,
    CONSTRAINT ck_artifact_checksum CHECK (((checksum_sha256 IS NULL) OR (checksum_sha256 ~ '^[0-9a-fA-F]{64}$'::text))),
    CONSTRAINT ck_artifact_size CHECK (((file_size_bytes IS NULL) OR (file_size_bytes >= 0)))
);


ALTER TABLE app.report_artifacts OWNER TO postgres;

--
-- TOC entry 245 (class 1259 OID 17723)
-- Name: report_requests; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.report_requests (
    report_request_id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    report_type character varying(50) NOT NULL,
    parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    status character varying(20) DEFAULT 'queued'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    error_message text,
    CONSTRAINT ck_report_parameters_object CHECK ((jsonb_typeof(parameters) = 'object'::text)),
    CONSTRAINT ck_report_status CHECK (((status)::text = ANY ((ARRAY['queued'::character varying, 'running'::character varying, 'completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))
);


ALTER TABLE app.report_requests OWNER TO postgres;

--
-- TOC entry 244 (class 1259 OID 17690)
-- Name: saved_views; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.saved_views (
    saved_view_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    project_id uuid,
    name character varying(120) NOT NULL,
    resource_filters jsonb DEFAULT '{}'::jsonb NOT NULL,
    metric_filters jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_saved_view_filters_object CHECK (((jsonb_typeof(resource_filters) = 'object'::text) AND (jsonb_typeof(metric_filters) = 'object'::text)))
);


ALTER TABLE app.saved_views OWNER TO postgres;

--
-- TOC entry 240 (class 1259 OID 17566)
-- Name: user_preferences; Type: TABLE; Schema: app; Owner: postgres
--

CREATE TABLE app.user_preferences (
    user_id uuid NOT NULL,
    timezone character varying(64) DEFAULT 'UTC'::character varying NOT NULL,
    locale character varying(20) DEFAULT 'es-CO'::character varying NOT NULL,
    currency character(3) DEFAULT 'USD'::bpchar NOT NULL,
    theme character varying(20) DEFAULT 'system'::character varying NOT NULL,
    notification_settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_user_preferences_currency CHECK ((currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT ck_user_preferences_notification_object CHECK ((jsonb_typeof(notification_settings) = 'object'::text)),
    CONSTRAINT ck_user_preferences_theme CHECK (((theme)::text = ANY ((ARRAY['system'::character varying, 'light'::character varying, 'dark'::character varying])::text[])))
);


ALTER TABLE app.user_preferences OWNER TO postgres;

--
-- TOC entry 249 (class 1259 OID 17809)
-- Name: events; Type: TABLE; Schema: audit; Owner: postgres
--

CREATE TABLE audit.events (
    event_id bigint NOT NULL,
    organization_id uuid,
    user_id uuid,
    action character varying(80) NOT NULL,
    entity_type character varying(80),
    entity_id uuid,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    request_id uuid,
    ip_address inet,
    user_agent text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT ck_audit_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text))
);


ALTER TABLE audit.events OWNER TO postgres;

--
-- TOC entry 248 (class 1259 OID 17808)
-- Name: events_event_id_seq; Type: SEQUENCE; Schema: audit; Owner: postgres
--

ALTER TABLE audit.events ALTER COLUMN event_id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME audit.events_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 238 (class 1259 OID 17514)
-- Name: account_projects; Type: TABLE; Schema: cloud; Owner: postgres
--

CREATE TABLE cloud.account_projects (
    cloud_account_id uuid NOT NULL,
    project_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE cloud.account_projects OWNER TO postgres;

--
-- TOC entry 237 (class 1259 OID 17478)
-- Name: accounts; Type: TABLE; Schema: cloud; Owner: postgres
--

CREATE TABLE cloud.accounts (
    cloud_account_id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    provider_code character varying(30) NOT NULL,
    display_name character varying(160) NOT NULL,
    subscription_id character varying(128) NOT NULL,
    tenant_id character varying(128),
    credential_reference character varying(512),
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_cloud_accounts_display_name CHECK (((length(btrim((display_name)::text)) >= 1) AND (length(btrim((display_name)::text)) <= 160))),
    CONSTRAINT ck_cloud_accounts_provider CHECK (((provider_code)::text = ANY ((ARRAY['azure'::character varying, 'aws'::character varying, 'gcp'::character varying])::text[]))),
    CONSTRAINT ck_cloud_accounts_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'active'::character varying, 'error'::character varying, 'disabled'::character varying, 'deleted'::character varying])::text[])))
);


ALTER TABLE cloud.accounts OWNER TO postgres;

--
-- TOC entry 239 (class 1259 OID 17534)
-- Name: resource_bindings; Type: TABLE; Schema: cloud; Owner: postgres
--

CREATE TABLE cloud.resource_bindings (
    resource_binding_id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    cloud_account_id uuid NOT NULL,
    data_resource_uid uuid NOT NULL,
    display_name character varying(160),
    is_monitored boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE cloud.resource_bindings OWNER TO postgres;

--
-- TOC entry 227 (class 1259 OID 17301)
-- Name: external_identities; Type: TABLE; Schema: iam; Owner: postgres
--

CREATE TABLE iam.external_identities (
    external_identity_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider character varying(50) NOT NULL,
    subject character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone
);


ALTER TABLE iam.external_identities OWNER TO postgres;

--
-- TOC entry 231 (class 1259 OID 17337)
-- Name: permissions; Type: TABLE; Schema: iam; Owner: postgres
--

CREATE TABLE iam.permissions (
    permission_id smallint NOT NULL,
    permission_code character varying(100) NOT NULL,
    description character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE iam.permissions OWNER TO postgres;

--
-- TOC entry 230 (class 1259 OID 17336)
-- Name: permissions_permission_id_seq; Type: SEQUENCE; Schema: iam; Owner: postgres
--

CREATE SEQUENCE iam.permissions_permission_id_seq
    AS smallint
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE iam.permissions_permission_id_seq OWNER TO postgres;

--
-- TOC entry 5393 (class 0 OID 0)
-- Dependencies: 230
-- Name: permissions_permission_id_seq; Type: SEQUENCE OWNED BY; Schema: iam; Owner: postgres
--

ALTER SEQUENCE iam.permissions_permission_id_seq OWNED BY iam.permissions.permission_id;


--
-- TOC entry 232 (class 1259 OID 17349)
-- Name: role_permissions; Type: TABLE; Schema: iam; Owner: postgres
--

CREATE TABLE iam.role_permissions (
    role_id smallint NOT NULL,
    permission_id smallint NOT NULL
);


ALTER TABLE iam.role_permissions OWNER TO postgres;

--
-- TOC entry 229 (class 1259 OID 17322)
-- Name: roles; Type: TABLE; Schema: iam; Owner: postgres
--

CREATE TABLE iam.roles (
    role_id smallint NOT NULL,
    role_code character varying(50) NOT NULL,
    description character varying(255),
    is_system_role boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE iam.roles OWNER TO postgres;

--
-- TOC entry 228 (class 1259 OID 17321)
-- Name: roles_role_id_seq; Type: SEQUENCE; Schema: iam; Owner: postgres
--

CREATE SEQUENCE iam.roles_role_id_seq
    AS smallint
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE iam.roles_role_id_seq OWNER TO postgres;

--
-- TOC entry 5394 (class 0 OID 0)
-- Dependencies: 228
-- Name: roles_role_id_seq; Type: SEQUENCE OWNED BY; Schema: iam; Owner: postgres
--

ALTER SEQUENCE iam.roles_role_id_seq OWNED BY iam.roles.role_id;


--
-- TOC entry 233 (class 1259 OID 17367)
-- Name: user_roles; Type: TABLE; Schema: iam; Owner: postgres
--

CREATE TABLE iam.user_roles (
    user_id uuid NOT NULL,
    role_id smallint NOT NULL,
    assigned_at timestamp with time zone DEFAULT now() NOT NULL,
    assigned_by uuid
);


ALTER TABLE iam.user_roles OWNER TO postgres;

--
-- TOC entry 226 (class 1259 OID 17278)
-- Name: users; Type: TABLE; Schema: iam; Owner: postgres
--

CREATE TABLE iam.users (
    user_id uuid DEFAULT gen_random_uuid() NOT NULL,
    email public.citext NOT NULL,
    display_name character varying(160) NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    email_verified boolean DEFAULT false NOT NULL,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_users_display_name CHECK (((length(btrim((display_name)::text)) >= 1) AND (length(btrim((display_name)::text)) <= 160))),
    CONSTRAINT ck_users_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'active'::character varying, 'suspended'::character varying, 'deleted'::character varying])::text[])))
);


ALTER TABLE iam.users OWNER TO postgres;

--
-- TOC entry 235 (class 1259 OID 17413)
-- Name: members; Type: TABLE; Schema: organization; Owner: postgres
--

CREATE TABLE organization.members (
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    membership_role character varying(30) DEFAULT 'member'::character varying NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    joined_at timestamp with time zone DEFAULT now() NOT NULL,
    invited_by uuid,
    CONSTRAINT ck_members_role CHECK (((membership_role)::text = ANY ((ARRAY['owner'::character varying, 'admin'::character varying, 'member'::character varying, 'viewer'::character varying])::text[]))),
    CONSTRAINT ck_members_status CHECK (((status)::text = ANY ((ARRAY['invited'::character varying, 'active'::character varying, 'suspended'::character varying, 'removed'::character varying])::text[])))
);


ALTER TABLE organization.members OWNER TO postgres;

--
-- TOC entry 234 (class 1259 OID 17392)
-- Name: organizations; Type: TABLE; Schema: organization; Owner: postgres
--

CREATE TABLE organization.organizations (
    organization_id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(160) NOT NULL,
    slug character varying(80) NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_organizations_name CHECK (((length(btrim((name)::text)) >= 1) AND (length(btrim((name)::text)) <= 160))),
    CONSTRAINT ck_organizations_slug CHECK (((slug)::text ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'::text)),
    CONSTRAINT ck_organizations_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'suspended'::character varying, 'deleted'::character varying])::text[])))
);


ALTER TABLE organization.organizations OWNER TO postgres;

--
-- TOC entry 236 (class 1259 OID 17445)
-- Name: projects; Type: TABLE; Schema: organization; Owner: postgres
--

CREATE TABLE organization.projects (
    project_id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    name character varying(160) NOT NULL,
    description text,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_projects_name CHECK (((length(btrim((name)::text)) >= 1) AND (length(btrim((name)::text)) <= 160))),
    CONSTRAINT ck_projects_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'archived'::character varying, 'deleted'::character varying])::text[])))
);


ALTER TABLE organization.projects OWNER TO postgres;

--
-- TOC entry 4985 (class 2604 OID 17340)
-- Name: permissions permission_id; Type: DEFAULT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.permissions ALTER COLUMN permission_id SET DEFAULT nextval('iam.permissions_permission_id_seq'::regclass);


--
-- TOC entry 4982 (class 2604 OID 17325)
-- Name: roles role_id; Type: DEFAULT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.roles ALTER COLUMN role_id SET DEFAULT nextval('iam.roles_role_id_seq'::regclass);


--
-- TOC entry 5129 (class 2606 OID 17618)
-- Name: dashboard_preferences dashboard_preferences_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.dashboard_preferences
    ADD CONSTRAINT dashboard_preferences_pkey PRIMARY KEY (dashboard_preference_id);


--
-- TOC entry 5141 (class 2606 OID 17681)
-- Name: notification_deliveries notification_deliveries_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.notification_deliveries
    ADD CONSTRAINT notification_deliveries_pkey PRIMARY KEY (notification_delivery_id);


--
-- TOC entry 5138 (class 2606 OID 17650)
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (notification_id);


--
-- TOC entry 5162 (class 2606 OID 17794)
-- Name: recommendation_interactions recommendation_interactions_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.recommendation_interactions
    ADD CONSTRAINT recommendation_interactions_pkey PRIMARY KEY (recommendation_interaction_id);


--
-- TOC entry 5157 (class 2606 OID 17771)
-- Name: report_artifacts report_artifacts_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.report_artifacts
    ADD CONSTRAINT report_artifacts_pkey PRIMARY KEY (report_artifact_id);


--
-- TOC entry 5153 (class 2606 OID 17742)
-- Name: report_requests report_requests_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.report_requests
    ADD CONSTRAINT report_requests_pkey PRIMARY KEY (report_request_id);


--
-- TOC entry 5146 (class 2606 OID 17709)
-- Name: saved_views saved_views_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.saved_views
    ADD CONSTRAINT saved_views_pkey PRIMARY KEY (saved_view_id);


--
-- TOC entry 5143 (class 2606 OID 17683)
-- Name: notification_deliveries uq_notification_channel; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.notification_deliveries
    ADD CONSTRAINT uq_notification_channel UNIQUE (notification_id, channel);


--
-- TOC entry 5127 (class 2606 OID 17590)
-- Name: user_preferences user_preferences_pkey; Type: CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.user_preferences
    ADD CONSTRAINT user_preferences_pkey PRIMARY KEY (user_id);


--
-- TOC entry 5164 (class 2606 OID 17822)
-- Name: events events_pkey; Type: CONSTRAINT; Schema: audit; Owner: postgres
--

ALTER TABLE ONLY audit.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (event_id);


--
-- TOC entry 5116 (class 2606 OID 17522)
-- Name: account_projects account_projects_pkey; Type: CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.account_projects
    ADD CONSTRAINT account_projects_pkey PRIMARY KEY (cloud_account_id, project_id);


--
-- TOC entry 5110 (class 2606 OID 17499)
-- Name: accounts accounts_pkey; Type: CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.accounts
    ADD CONSTRAINT accounts_pkey PRIMARY KEY (cloud_account_id);


--
-- TOC entry 5121 (class 2606 OID 17549)
-- Name: resource_bindings resource_bindings_pkey; Type: CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.resource_bindings
    ADD CONSTRAINT resource_bindings_pkey PRIMARY KEY (resource_binding_id);


--
-- TOC entry 5114 (class 2606 OID 17501)
-- Name: accounts uq_cloud_account_subscription; Type: CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.accounts
    ADD CONSTRAINT uq_cloud_account_subscription UNIQUE (provider_code, subscription_id);


--
-- TOC entry 5123 (class 2606 OID 17551)
-- Name: resource_bindings uq_resource_binding; Type: CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.resource_bindings
    ADD CONSTRAINT uq_resource_binding UNIQUE (project_id, data_resource_uid);


--
-- TOC entry 5125 (class 2606 OID 17553)
-- Name: resource_bindings uq_resource_binding_account; Type: CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.resource_bindings
    ADD CONSTRAINT uq_resource_binding_account UNIQUE (cloud_account_id, data_resource_uid);


--
-- TOC entry 5076 (class 2606 OID 17312)
-- Name: external_identities external_identities_pkey; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.external_identities
    ADD CONSTRAINT external_identities_pkey PRIMARY KEY (external_identity_id);


--
-- TOC entry 5085 (class 2606 OID 17348)
-- Name: permissions permissions_permission_code_key; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.permissions
    ADD CONSTRAINT permissions_permission_code_key UNIQUE (permission_code);


--
-- TOC entry 5087 (class 2606 OID 17346)
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (permission_id);


--
-- TOC entry 5090 (class 2606 OID 17355)
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- TOC entry 5081 (class 2606 OID 17333)
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (role_id);


--
-- TOC entry 5083 (class 2606 OID 17335)
-- Name: roles roles_role_code_key; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.roles
    ADD CONSTRAINT roles_role_code_key UNIQUE (role_code);


--
-- TOC entry 5079 (class 2606 OID 17314)
-- Name: external_identities uq_external_identity; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.external_identities
    ADD CONSTRAINT uq_external_identity UNIQUE (provider, subject);


--
-- TOC entry 5093 (class 2606 OID 17375)
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);


--
-- TOC entry 5074 (class 2606 OID 17298)
-- Name: users users_pkey; Type: CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (user_id);


--
-- TOC entry 5102 (class 2606 OID 17427)
-- Name: members members_pkey; Type: CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.members
    ADD CONSTRAINT members_pkey PRIMARY KEY (organization_id, user_id);


--
-- TOC entry 5096 (class 2606 OID 17409)
-- Name: organizations organizations_pkey; Type: CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (organization_id);


--
-- TOC entry 5106 (class 2606 OID 17463)
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (project_id);


--
-- TOC entry 5098 (class 2606 OID 17411)
-- Name: organizations uq_organizations_slug; Type: CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.organizations
    ADD CONSTRAINT uq_organizations_slug UNIQUE (slug);


--
-- TOC entry 5108 (class 2606 OID 17465)
-- Name: projects uq_projects_org_name; Type: CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.projects
    ADD CONSTRAINT uq_projects_org_name UNIQUE (organization_id, name);


--
-- TOC entry 5130 (class 1259 OID 17632)
-- Name: ix_dashboard_preferences_project; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_dashboard_preferences_project ON app.dashboard_preferences USING btree (project_id);


--
-- TOC entry 5131 (class 1259 OID 17631)
-- Name: ix_dashboard_preferences_user; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_dashboard_preferences_user ON app.dashboard_preferences USING btree (user_id);


--
-- TOC entry 5139 (class 1259 OID 17689)
-- Name: ix_notification_deliveries_pending; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_notification_deliveries_pending ON app.notification_deliveries USING btree (status, last_attempt_at);


--
-- TOC entry 5134 (class 1259 OID 17662)
-- Name: ix_notifications_org_created; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_notifications_org_created ON app.notifications USING btree (organization_id, created_at DESC);


--
-- TOC entry 5135 (class 1259 OID 17663)
-- Name: ix_notifications_reference; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_notifications_reference ON app.notifications USING btree (reference_type, reference_id);


--
-- TOC entry 5136 (class 1259 OID 17661)
-- Name: ix_notifications_user_unread; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_notifications_user_unread ON app.notifications USING btree (user_id, created_at DESC) WHERE (read_at IS NULL);


--
-- TOC entry 5158 (class 1259 OID 17806)
-- Name: ix_recommendation_interactions_project_created; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_recommendation_interactions_project_created ON app.recommendation_interactions USING btree (project_id, created_at DESC);


--
-- TOC entry 5159 (class 1259 OID 17807)
-- Name: ix_recommendation_interactions_recommendation; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_recommendation_interactions_recommendation ON app.recommendation_interactions USING btree (recommendation_uid, created_at DESC);


--
-- TOC entry 5160 (class 1259 OID 17805)
-- Name: ix_recommendation_interactions_user_created; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_recommendation_interactions_user_created ON app.recommendation_interactions USING btree (user_id, created_at DESC);


--
-- TOC entry 5154 (class 1259 OID 17778)
-- Name: ix_report_artifacts_expiry; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_report_artifacts_expiry ON app.report_artifacts USING btree (expires_at) WHERE (expires_at IS NOT NULL);


--
-- TOC entry 5155 (class 1259 OID 17777)
-- Name: ix_report_artifacts_request; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_report_artifacts_request ON app.report_artifacts USING btree (report_request_id);


--
-- TOC entry 5149 (class 1259 OID 17753)
-- Name: ix_report_requests_project_created; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_report_requests_project_created ON app.report_requests USING btree (project_id, created_at DESC);


--
-- TOC entry 5150 (class 1259 OID 17755)
-- Name: ix_report_requests_queue; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_report_requests_queue ON app.report_requests USING btree (status, created_at) WHERE ((status)::text = ANY ((ARRAY['queued'::character varying, 'running'::character varying])::text[]));


--
-- TOC entry 5151 (class 1259 OID 17754)
-- Name: ix_report_requests_user_created; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_report_requests_user_created ON app.report_requests USING btree (requested_by, created_at DESC);


--
-- TOC entry 5144 (class 1259 OID 17720)
-- Name: ix_saved_views_user_project; Type: INDEX; Schema: app; Owner: postgres
--

CREATE INDEX ix_saved_views_user_project ON app.saved_views USING btree (user_id, project_id);


--
-- TOC entry 5132 (class 1259 OID 17629)
-- Name: uq_dashboard_preferences_global_user; Type: INDEX; Schema: app; Owner: postgres
--

CREATE UNIQUE INDEX uq_dashboard_preferences_global_user ON app.dashboard_preferences USING btree (user_id) WHERE (project_id IS NULL);


--
-- TOC entry 5133 (class 1259 OID 17630)
-- Name: uq_dashboard_preferences_project_user; Type: INDEX; Schema: app; Owner: postgres
--

CREATE UNIQUE INDEX uq_dashboard_preferences_project_user ON app.dashboard_preferences USING btree (user_id, project_id) WHERE (project_id IS NOT NULL);


--
-- TOC entry 5147 (class 1259 OID 17722)
-- Name: uq_saved_views_user_global_name; Type: INDEX; Schema: app; Owner: postgres
--

CREATE UNIQUE INDEX uq_saved_views_user_global_name ON app.saved_views USING btree (user_id, name) WHERE (project_id IS NULL);


--
-- TOC entry 5148 (class 1259 OID 17721)
-- Name: uq_saved_views_user_project_name; Type: INDEX; Schema: app; Owner: postgres
--

CREATE UNIQUE INDEX uq_saved_views_user_project_name ON app.saved_views USING btree (user_id, project_id, name) WHERE (project_id IS NOT NULL);


--
-- TOC entry 5165 (class 1259 OID 17836)
-- Name: ix_audit_action_time; Type: INDEX; Schema: audit; Owner: postgres
--

CREATE INDEX ix_audit_action_time ON audit.events USING btree (action, occurred_at DESC);


--
-- TOC entry 5166 (class 1259 OID 17835)
-- Name: ix_audit_entity_time; Type: INDEX; Schema: audit; Owner: postgres
--

CREATE INDEX ix_audit_entity_time ON audit.events USING btree (entity_type, entity_id, occurred_at DESC);


--
-- TOC entry 5167 (class 1259 OID 17833)
-- Name: ix_audit_org_time; Type: INDEX; Schema: audit; Owner: postgres
--

CREATE INDEX ix_audit_org_time ON audit.events USING btree (organization_id, occurred_at DESC);


--
-- TOC entry 5168 (class 1259 OID 17837)
-- Name: ix_audit_request_id; Type: INDEX; Schema: audit; Owner: postgres
--

CREATE INDEX ix_audit_request_id ON audit.events USING btree (request_id) WHERE (request_id IS NOT NULL);


--
-- TOC entry 5169 (class 1259 OID 17834)
-- Name: ix_audit_user_time; Type: INDEX; Schema: audit; Owner: postgres
--

CREATE INDEX ix_audit_user_time ON audit.events USING btree (user_id, occurred_at DESC);


--
-- TOC entry 5117 (class 1259 OID 17533)
-- Name: ix_account_projects_project; Type: INDEX; Schema: cloud; Owner: postgres
--

CREATE INDEX ix_account_projects_project ON cloud.account_projects USING btree (project_id);


--
-- TOC entry 5111 (class 1259 OID 17513)
-- Name: ix_cloud_accounts_created_by; Type: INDEX; Schema: cloud; Owner: postgres
--

CREATE INDEX ix_cloud_accounts_created_by ON cloud.accounts USING btree (created_by);


--
-- TOC entry 5112 (class 1259 OID 17512)
-- Name: ix_cloud_accounts_org_status; Type: INDEX; Schema: cloud; Owner: postgres
--

CREATE INDEX ix_cloud_accounts_org_status ON cloud.accounts USING btree (organization_id, status);


--
-- TOC entry 5118 (class 1259 OID 17565)
-- Name: ix_resource_bindings_data_uid; Type: INDEX; Schema: cloud; Owner: postgres
--

CREATE INDEX ix_resource_bindings_data_uid ON cloud.resource_bindings USING btree (data_resource_uid);


--
-- TOC entry 5119 (class 1259 OID 17564)
-- Name: ix_resource_bindings_project_monitored; Type: INDEX; Schema: cloud; Owner: postgres
--

CREATE INDEX ix_resource_bindings_project_monitored ON cloud.resource_bindings USING btree (project_id, is_monitored);


--
-- TOC entry 5077 (class 1259 OID 17320)
-- Name: ix_external_identities_user; Type: INDEX; Schema: iam; Owner: postgres
--

CREATE INDEX ix_external_identities_user ON iam.external_identities USING btree (user_id);


--
-- TOC entry 5088 (class 1259 OID 17366)
-- Name: ix_role_permissions_permission; Type: INDEX; Schema: iam; Owner: postgres
--

CREATE INDEX ix_role_permissions_permission ON iam.role_permissions USING btree (permission_id);


--
-- TOC entry 5091 (class 1259 OID 17391)
-- Name: ix_user_roles_role; Type: INDEX; Schema: iam; Owner: postgres
--

CREATE INDEX ix_user_roles_role ON iam.user_roles USING btree (role_id);


--
-- TOC entry 5071 (class 1259 OID 17300)
-- Name: ix_users_status_created; Type: INDEX; Schema: iam; Owner: postgres
--

CREATE INDEX ix_users_status_created ON iam.users USING btree (status, created_at DESC);


--
-- TOC entry 5072 (class 1259 OID 17299)
-- Name: uq_users_email; Type: INDEX; Schema: iam; Owner: postgres
--

CREATE UNIQUE INDEX uq_users_email ON iam.users USING btree (email);


--
-- TOC entry 5099 (class 1259 OID 17444)
-- Name: ix_members_org_status; Type: INDEX; Schema: organization; Owner: postgres
--

CREATE INDEX ix_members_org_status ON organization.members USING btree (organization_id, status);


--
-- TOC entry 5100 (class 1259 OID 17443)
-- Name: ix_members_user_status; Type: INDEX; Schema: organization; Owner: postgres
--

CREATE INDEX ix_members_user_status ON organization.members USING btree (user_id, status);


--
-- TOC entry 5094 (class 1259 OID 17412)
-- Name: ix_organizations_status; Type: INDEX; Schema: organization; Owner: postgres
--

CREATE INDEX ix_organizations_status ON organization.organizations USING btree (status);


--
-- TOC entry 5103 (class 1259 OID 17477)
-- Name: ix_projects_created_by; Type: INDEX; Schema: organization; Owner: postgres
--

CREATE INDEX ix_projects_created_by ON organization.projects USING btree (created_by);


--
-- TOC entry 5104 (class 1259 OID 17476)
-- Name: ix_projects_org_status; Type: INDEX; Schema: organization; Owner: postgres
--

CREATE INDEX ix_projects_org_status ON organization.projects USING btree (organization_id, status);


--
-- TOC entry 5208 (class 2620 OID 17845)
-- Name: dashboard_preferences trg_dashboard_preferences_updated_at; Type: TRIGGER; Schema: app; Owner: postgres
--

CREATE TRIGGER trg_dashboard_preferences_updated_at BEFORE UPDATE ON app.dashboard_preferences FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5209 (class 2620 OID 17846)
-- Name: saved_views trg_saved_views_updated_at; Type: TRIGGER; Schema: app; Owner: postgres
--

CREATE TRIGGER trg_saved_views_updated_at BEFORE UPDATE ON app.saved_views FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5207 (class 2620 OID 17844)
-- Name: user_preferences trg_user_preferences_updated_at; Type: TRIGGER; Schema: app; Owner: postgres
--

CREATE TRIGGER trg_user_preferences_updated_at BEFORE UPDATE ON app.user_preferences FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5205 (class 2620 OID 17842)
-- Name: accounts trg_cloud_accounts_updated_at; Type: TRIGGER; Schema: cloud; Owner: postgres
--

CREATE TRIGGER trg_cloud_accounts_updated_at BEFORE UPDATE ON cloud.accounts FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5206 (class 2620 OID 17843)
-- Name: resource_bindings trg_resource_bindings_updated_at; Type: TRIGGER; Schema: cloud; Owner: postgres
--

CREATE TRIGGER trg_resource_bindings_updated_at BEFORE UPDATE ON cloud.resource_bindings FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5202 (class 2620 OID 17839)
-- Name: users trg_users_updated_at; Type: TRIGGER; Schema: iam; Owner: postgres
--

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON iam.users FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5203 (class 2620 OID 17840)
-- Name: organizations trg_organizations_updated_at; Type: TRIGGER; Schema: organization; Owner: postgres
--

CREATE TRIGGER trg_organizations_updated_at BEFORE UPDATE ON organization.organizations FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5204 (class 2620 OID 17841)
-- Name: projects trg_projects_updated_at; Type: TRIGGER; Schema: organization; Owner: postgres
--

CREATE TRIGGER trg_projects_updated_at BEFORE UPDATE ON organization.projects FOR EACH ROW EXECUTE FUNCTION app.set_updated_at();


--
-- TOC entry 5188 (class 2606 OID 17624)
-- Name: dashboard_preferences dashboard_preferences_project_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.dashboard_preferences
    ADD CONSTRAINT dashboard_preferences_project_id_fkey FOREIGN KEY (project_id) REFERENCES organization.projects(project_id) ON DELETE CASCADE;


--
-- TOC entry 5189 (class 2606 OID 17619)
-- Name: dashboard_preferences dashboard_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.dashboard_preferences
    ADD CONSTRAINT dashboard_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5192 (class 2606 OID 17684)
-- Name: notification_deliveries notification_deliveries_notification_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.notification_deliveries
    ADD CONSTRAINT notification_deliveries_notification_id_fkey FOREIGN KEY (notification_id) REFERENCES app.notifications(notification_id) ON DELETE CASCADE;


--
-- TOC entry 5190 (class 2606 OID 17656)
-- Name: notifications notifications_organization_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.notifications
    ADD CONSTRAINT notifications_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organization.organizations(organization_id) ON DELETE CASCADE;


--
-- TOC entry 5191 (class 2606 OID 17651)
-- Name: notifications notifications_user_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.notifications
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5198 (class 2606 OID 17800)
-- Name: recommendation_interactions recommendation_interactions_project_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.recommendation_interactions
    ADD CONSTRAINT recommendation_interactions_project_id_fkey FOREIGN KEY (project_id) REFERENCES organization.projects(project_id) ON DELETE CASCADE;


--
-- TOC entry 5199 (class 2606 OID 17795)
-- Name: recommendation_interactions recommendation_interactions_user_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.recommendation_interactions
    ADD CONSTRAINT recommendation_interactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE RESTRICT;


--
-- TOC entry 5197 (class 2606 OID 17772)
-- Name: report_artifacts report_artifacts_report_request_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.report_artifacts
    ADD CONSTRAINT report_artifacts_report_request_id_fkey FOREIGN KEY (report_request_id) REFERENCES app.report_requests(report_request_id) ON DELETE CASCADE;


--
-- TOC entry 5195 (class 2606 OID 17743)
-- Name: report_requests report_requests_project_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.report_requests
    ADD CONSTRAINT report_requests_project_id_fkey FOREIGN KEY (project_id) REFERENCES organization.projects(project_id) ON DELETE CASCADE;


--
-- TOC entry 5196 (class 2606 OID 17748)
-- Name: report_requests report_requests_requested_by_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.report_requests
    ADD CONSTRAINT report_requests_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES iam.users(user_id) ON DELETE RESTRICT;


--
-- TOC entry 5193 (class 2606 OID 17715)
-- Name: saved_views saved_views_project_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.saved_views
    ADD CONSTRAINT saved_views_project_id_fkey FOREIGN KEY (project_id) REFERENCES organization.projects(project_id) ON DELETE CASCADE;


--
-- TOC entry 5194 (class 2606 OID 17710)
-- Name: saved_views saved_views_user_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.saved_views
    ADD CONSTRAINT saved_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5187 (class 2606 OID 17591)
-- Name: user_preferences user_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: app; Owner: postgres
--

ALTER TABLE ONLY app.user_preferences
    ADD CONSTRAINT user_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5200 (class 2606 OID 17823)
-- Name: events events_organization_id_fkey; Type: FK CONSTRAINT; Schema: audit; Owner: postgres
--

ALTER TABLE ONLY audit.events
    ADD CONSTRAINT events_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organization.organizations(organization_id) ON DELETE SET NULL;


--
-- TOC entry 5201 (class 2606 OID 17828)
-- Name: events events_user_id_fkey; Type: FK CONSTRAINT; Schema: audit; Owner: postgres
--

ALTER TABLE ONLY audit.events
    ADD CONSTRAINT events_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE SET NULL;


--
-- TOC entry 5183 (class 2606 OID 17523)
-- Name: account_projects account_projects_cloud_account_id_fkey; Type: FK CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.account_projects
    ADD CONSTRAINT account_projects_cloud_account_id_fkey FOREIGN KEY (cloud_account_id) REFERENCES cloud.accounts(cloud_account_id) ON DELETE CASCADE;


--
-- TOC entry 5184 (class 2606 OID 17528)
-- Name: account_projects account_projects_project_id_fkey; Type: FK CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.account_projects
    ADD CONSTRAINT account_projects_project_id_fkey FOREIGN KEY (project_id) REFERENCES organization.projects(project_id) ON DELETE CASCADE;


--
-- TOC entry 5181 (class 2606 OID 17507)
-- Name: accounts accounts_created_by_fkey; Type: FK CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.accounts
    ADD CONSTRAINT accounts_created_by_fkey FOREIGN KEY (created_by) REFERENCES iam.users(user_id) ON DELETE SET NULL;


--
-- TOC entry 5182 (class 2606 OID 17502)
-- Name: accounts accounts_organization_id_fkey; Type: FK CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.accounts
    ADD CONSTRAINT accounts_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organization.organizations(organization_id) ON DELETE CASCADE;


--
-- TOC entry 5185 (class 2606 OID 17559)
-- Name: resource_bindings resource_bindings_cloud_account_id_fkey; Type: FK CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.resource_bindings
    ADD CONSTRAINT resource_bindings_cloud_account_id_fkey FOREIGN KEY (cloud_account_id) REFERENCES cloud.accounts(cloud_account_id) ON DELETE CASCADE;


--
-- TOC entry 5186 (class 2606 OID 17554)
-- Name: resource_bindings resource_bindings_project_id_fkey; Type: FK CONSTRAINT; Schema: cloud; Owner: postgres
--

ALTER TABLE ONLY cloud.resource_bindings
    ADD CONSTRAINT resource_bindings_project_id_fkey FOREIGN KEY (project_id) REFERENCES organization.projects(project_id) ON DELETE CASCADE;


--
-- TOC entry 5170 (class 2606 OID 17315)
-- Name: external_identities external_identities_user_id_fkey; Type: FK CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.external_identities
    ADD CONSTRAINT external_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5171 (class 2606 OID 17361)
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES iam.permissions(permission_id) ON DELETE CASCADE;


--
-- TOC entry 5172 (class 2606 OID 17356)
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES iam.roles(role_id) ON DELETE CASCADE;


--
-- TOC entry 5173 (class 2606 OID 17386)
-- Name: user_roles user_roles_assigned_by_fkey; Type: FK CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.user_roles
    ADD CONSTRAINT user_roles_assigned_by_fkey FOREIGN KEY (assigned_by) REFERENCES iam.users(user_id) ON DELETE SET NULL;


--
-- TOC entry 5174 (class 2606 OID 17381)
-- Name: user_roles user_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.user_roles
    ADD CONSTRAINT user_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES iam.roles(role_id) ON DELETE RESTRICT;


--
-- TOC entry 5175 (class 2606 OID 17376)
-- Name: user_roles user_roles_user_id_fkey; Type: FK CONSTRAINT; Schema: iam; Owner: postgres
--

ALTER TABLE ONLY iam.user_roles
    ADD CONSTRAINT user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5176 (class 2606 OID 17438)
-- Name: members members_invited_by_fkey; Type: FK CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.members
    ADD CONSTRAINT members_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES iam.users(user_id) ON DELETE SET NULL;


--
-- TOC entry 5177 (class 2606 OID 17428)
-- Name: members members_organization_id_fkey; Type: FK CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.members
    ADD CONSTRAINT members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organization.organizations(organization_id) ON DELETE CASCADE;


--
-- TOC entry 5178 (class 2606 OID 17433)
-- Name: members members_user_id_fkey; Type: FK CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.members
    ADD CONSTRAINT members_user_id_fkey FOREIGN KEY (user_id) REFERENCES iam.users(user_id) ON DELETE CASCADE;


--
-- TOC entry 5179 (class 2606 OID 17471)
-- Name: projects projects_created_by_fkey; Type: FK CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.projects
    ADD CONSTRAINT projects_created_by_fkey FOREIGN KEY (created_by) REFERENCES iam.users(user_id) ON DELETE SET NULL;


--
-- TOC entry 5180 (class 2606 OID 17466)
-- Name: projects projects_organization_id_fkey; Type: FK CONSTRAINT; Schema: organization; Owner: postgres
--

ALTER TABLE ONLY organization.projects
    ADD CONSTRAINT projects_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organization.organizations(organization_id) ON DELETE CASCADE;


--
-- TOC entry 5364 (class 0 OID 17596)
-- Dependencies: 241
-- Name: dashboard_preferences; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.dashboard_preferences ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5378 (class 3256 OID 17859)
-- Name: dashboard_preferences dashboard_preferences_self; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY dashboard_preferences_self ON app.dashboard_preferences USING (((user_id = app.current_user_id()) OR app.is_platform_admin())) WITH CHECK (((user_id = app.current_user_id()) OR app.is_platform_admin()));


--
-- TOC entry 5365 (class 0 OID 17633)
-- Dependencies: 242
-- Name: notifications; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.notifications ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5379 (class 3256 OID 17860)
-- Name: notifications notifications_self; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY notifications_self ON app.notifications USING (((user_id = app.current_user_id()) OR app.is_platform_admin())) WITH CHECK (((user_id = app.current_user_id()) OR app.is_platform_admin()));


--
-- TOC entry 5369 (class 0 OID 17779)
-- Dependencies: 247
-- Name: recommendation_interactions; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.recommendation_interactions ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5383 (class 3256 OID 17866)
-- Name: recommendation_interactions recommendation_interactions_project_access; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY recommendation_interactions_project_access ON app.recommendation_interactions USING ((app.is_platform_admin() OR (EXISTS ( SELECT 1
   FROM (organization.projects p
     JOIN organization.members m ON ((m.organization_id = p.organization_id)))
  WHERE ((p.project_id = recommendation_interactions.project_id) AND (m.user_id = app.current_user_id()) AND ((m.status)::text = 'active'::text)))))) WITH CHECK ((app.is_platform_admin() OR (user_id = app.current_user_id())));


--
-- TOC entry 5368 (class 0 OID 17756)
-- Dependencies: 246
-- Name: report_artifacts; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.report_artifacts ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5382 (class 3256 OID 17864)
-- Name: report_artifacts report_artifacts_project_access; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY report_artifacts_project_access ON app.report_artifacts USING ((app.is_platform_admin() OR (EXISTS ( SELECT 1
   FROM app.report_requests rr
  WHERE ((rr.report_request_id = report_artifacts.report_request_id) AND ((rr.requested_by = app.current_user_id()) OR (EXISTS ( SELECT 1
           FROM (organization.projects p
             JOIN organization.members m ON ((m.organization_id = p.organization_id)))
          WHERE ((p.project_id = rr.project_id) AND (m.user_id = app.current_user_id()) AND ((m.status)::text = 'active'::text))))))))));


--
-- TOC entry 5367 (class 0 OID 17723)
-- Dependencies: 245
-- Name: report_requests; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.report_requests ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5381 (class 3256 OID 17862)
-- Name: report_requests reports_project_access; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY reports_project_access ON app.report_requests USING ((app.is_platform_admin() OR (requested_by = app.current_user_id()) OR (EXISTS ( SELECT 1
   FROM (organization.projects p
     JOIN organization.members m ON ((m.organization_id = p.organization_id)))
  WHERE ((p.project_id = report_requests.project_id) AND (m.user_id = app.current_user_id()) AND ((m.status)::text = 'active'::text)))))) WITH CHECK ((app.is_platform_admin() OR (requested_by = app.current_user_id())));


--
-- TOC entry 5366 (class 0 OID 17690)
-- Dependencies: 244
-- Name: saved_views; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.saved_views ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5380 (class 3256 OID 17861)
-- Name: saved_views saved_views_self; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY saved_views_self ON app.saved_views USING (((user_id = app.current_user_id()) OR app.is_platform_admin())) WITH CHECK (((user_id = app.current_user_id()) OR app.is_platform_admin()));


--
-- TOC entry 5363 (class 0 OID 17566)
-- Dependencies: 240
-- Name: user_preferences; Type: ROW SECURITY; Schema: app; Owner: postgres
--

ALTER TABLE app.user_preferences ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5377 (class 3256 OID 17858)
-- Name: user_preferences user_preferences_self; Type: POLICY; Schema: app; Owner: postgres
--

CREATE POLICY user_preferences_self ON app.user_preferences USING (((user_id = app.current_user_id()) OR app.is_platform_admin())) WITH CHECK (((user_id = app.current_user_id()) OR app.is_platform_admin()));


--
-- TOC entry 5384 (class 3256 OID 17868)
-- Name: events audit_insert_own_context; Type: POLICY; Schema: audit; Owner: postgres
--

CREATE POLICY audit_insert_own_context ON audit.events FOR INSERT WITH CHECK ((app.is_platform_admin() OR ((user_id = app.current_user_id()) AND ((organization_id IS NULL) OR app.is_org_member(organization_id)))));


--
-- TOC entry 5385 (class 3256 OID 17869)
-- Name: events audit_read_own_org; Type: POLICY; Schema: audit; Owner: postgres
--

CREATE POLICY audit_read_own_org ON audit.events FOR SELECT USING ((app.is_platform_admin() OR (user_id = app.current_user_id()) OR (EXISTS ( SELECT 1
   FROM organization.members m
  WHERE ((m.organization_id = events.organization_id) AND (m.user_id = app.current_user_id()) AND ((m.status)::text = 'active'::text))))));


--
-- TOC entry 5370 (class 0 OID 17809)
-- Dependencies: 249
-- Name: events; Type: ROW SECURITY; Schema: audit; Owner: postgres
--

ALTER TABLE audit.events ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5361 (class 0 OID 17514)
-- Dependencies: 238
-- Name: account_projects; Type: ROW SECURITY; Schema: cloud; Owner: postgres
--

ALTER TABLE cloud.account_projects ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5375 (class 3256 OID 17855)
-- Name: account_projects account_projects_tenant_access; Type: POLICY; Schema: cloud; Owner: postgres
--

CREATE POLICY account_projects_tenant_access ON cloud.account_projects USING ((app.is_platform_admin() OR (EXISTS ( SELECT 1
   FROM cloud.accounts ca
  WHERE ((ca.cloud_account_id = account_projects.cloud_account_id) AND app.is_org_member(ca.organization_id)))))) WITH CHECK ((app.is_platform_admin() OR (EXISTS ( SELECT 1
   FROM cloud.accounts ca
  WHERE ((ca.cloud_account_id = account_projects.cloud_account_id) AND app.is_org_member(ca.organization_id))))));


--
-- TOC entry 5360 (class 0 OID 17478)
-- Dependencies: 237
-- Name: accounts; Type: ROW SECURITY; Schema: cloud; Owner: postgres
--

ALTER TABLE cloud.accounts ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5374 (class 3256 OID 17854)
-- Name: accounts cloud_accounts_tenant_access; Type: POLICY; Schema: cloud; Owner: postgres
--

CREATE POLICY cloud_accounts_tenant_access ON cloud.accounts USING ((app.is_platform_admin() OR app.is_org_member(organization_id))) WITH CHECK ((app.is_platform_admin() OR app.is_org_member(organization_id)));


--
-- TOC entry 5362 (class 0 OID 17534)
-- Dependencies: 239
-- Name: resource_bindings; Type: ROW SECURITY; Schema: cloud; Owner: postgres
--

ALTER TABLE cloud.resource_bindings ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5376 (class 3256 OID 17857)
-- Name: resource_bindings resource_bindings_tenant_access; Type: POLICY; Schema: cloud; Owner: postgres
--

CREATE POLICY resource_bindings_tenant_access ON cloud.resource_bindings USING ((app.is_platform_admin() OR app.is_project_member(project_id))) WITH CHECK ((app.is_platform_admin() OR app.is_project_member(project_id)));


--
-- TOC entry 5358 (class 0 OID 17413)
-- Dependencies: 235
-- Name: members; Type: ROW SECURITY; Schema: organization; Owner: postgres
--

ALTER TABLE organization.members ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5372 (class 3256 OID 17852)
-- Name: members members_select; Type: POLICY; Schema: organization; Owner: postgres
--

CREATE POLICY members_select ON organization.members FOR SELECT USING ((app.is_platform_admin() OR app.is_org_member(organization_id)));


--
-- TOC entry 5357 (class 0 OID 17392)
-- Dependencies: 234
-- Name: organizations; Type: ROW SECURITY; Schema: organization; Owner: postgres
--

ALTER TABLE organization.organizations ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5371 (class 3256 OID 17851)
-- Name: organizations organizations_select; Type: POLICY; Schema: organization; Owner: postgres
--

CREATE POLICY organizations_select ON organization.organizations FOR SELECT USING ((app.is_platform_admin() OR app.is_org_member(organization_id)));


--
-- TOC entry 5359 (class 0 OID 17445)
-- Dependencies: 236
-- Name: projects; Type: ROW SECURITY; Schema: organization; Owner: postgres
--

ALTER TABLE organization.projects ENABLE ROW LEVEL SECURITY;

--
-- TOC entry 5373 (class 3256 OID 17853)
-- Name: projects projects_tenant_access; Type: POLICY; Schema: organization; Owner: postgres
--

CREATE POLICY projects_tenant_access ON organization.projects USING ((app.is_platform_admin() OR app.is_org_member(organization_id))) WITH CHECK ((app.is_platform_admin() OR app.is_org_member(organization_id)));


-- Completed on 2026-09-27 21:11:40

--
-- PostgreSQL database dump complete
--

\unrestrict Sj7lMTD4WhhaYCnLeLJJzzl3YyCE0AtJtpAodaeoM88rBhibvHxf2Rb5MQBS9OG

