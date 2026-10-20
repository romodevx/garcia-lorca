"use client";

import { createContext, useContext } from "react";

import { STEPS, type StepDef } from "./setupSteps";

/*
 * The step list the setup view shows (stepsFor): the eight steps, or seven
 * with «Estructura de la escuela» (module_school_structure). CatalogSetup
 * provides it; every component that numbers steps or walks to a neighbour
 * reads it here — never STEPS directly — so both lists stay consistent.
 */
const SetupStepsContext = createContext<readonly StepDef[]>(STEPS);

export const SetupStepsProvider = SetupStepsContext.Provider;

export function useSetupSteps(): readonly StepDef[] {
  return useContext(SetupStepsContext);
}
