"use client";

import type { ReactElement } from "react";

import { Btn } from "../_design/Controls";
import { Icon } from "../_design/Icon";
import { Page } from "../_design/Page";
import { MODULES, isModuleVisible } from "../_design/data";
import { greetingFor, todayLabel } from "./greeting";
import { HomeKpis } from "./HomeKpis";
import { HomeModules, HomeModulesSkeleton } from "./HomeModules";
import { NoModules, PermissionsUnavailable } from "./HomeNotices";
import type { HomeProps } from "./types";
import { useHomeKpis } from "./useHomeKpis";

/**
 * The refreshed home, "option C" (PLAN-redesign-home §3): modules by area, as
 * cards with shortcuts, and KPIs that say when they do not know. Same contract
 * as the classic home (§2, B1–B12); only the layout and the KPI states change.
 */
export function HomeView({ user, onOpen, permissions, permissionsFailed = false, onRetryPermissions, now = new Date() }: HomeProps): ReactElement {
  const resolved = permissions !== undefined;
  const canSee = (perm: string): boolean => resolved && permissions.includes(perm);
  const visibleModules = resolved ? MODULES.filter((m) => isModuleVisible(m, permissions)) : [];

  // B6: each read only for a caller who may make it.
  const readsCycles = canSee("config:read");
  const kpis = useHomeKpis({ students: canSee("students:read"), cycles: readsCycles, ledger: readsCycles && canSee("finance:read") });

  return (
    <Page>
      <div className="home">
        <header className="home-hero">
          <div>
            <h1 className="home-greeting look-greeting">
              {greetingFor(now)}, {user.first} <Icon name="sprout" size={28} className="home-greeting__sprout" />
            </h1>
            <p className="home-date">
              {todayLabel(now)}
              {kpis.cycle !== undefined && ` · Ciclo ${kpis.cycle.name}`}
            </p>
          </div>
          {canSee("comms:read") && (
            <Btn icon="plus" kind="forest" onClick={() => onOpen("announcements")}>
              Nuevo aviso
            </Btn>
          )}
        </header>

        {resolved && <HomeKpis kpis={kpis} permissions={permissions} onOpen={onOpen} />}

        {permissionsFailed ? (
          <PermissionsUnavailable onRetry={onRetryPermissions} />
        ) : !resolved ? (
          <HomeModulesSkeleton />
        ) : visibleModules.length === 0 ? (
          <NoModules />
        ) : (
          <HomeModules modules={visibleModules} onOpen={onOpen} />
        )}
      </div>
    </Page>
  );
}
