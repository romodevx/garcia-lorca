"use client";

import type { CSSProperties, MouseEvent, ReactElement } from "react";

import { COLOR } from "../../_design/colors";
import type { ScheduleBlock } from "../../schedule/api";
import { blockLabel, WEEKDAYS, type ScheduleIndex } from "../../schedule/catalog";
import { layoutColumns, SLOT_MINUTES, snapToSlot, subjectColor, toHHMM, toMinutes } from "../../schedule/time";
import { card } from "./parts";

/** Height of one 5-minute row: an hour is 72 px, a 50-minute class 60 px. */
const ROW_PX = 6;
const HOUR = 60;
const GUTTER_PX = 56;
const DAY_MIN_PX = 150;
const HEADER_PX = 44;
const MIN_BLOCK_PX = 22;

const px = (minutes: number): number => (minutes / SLOT_MINUTES) * ROW_PX;

export interface WeekGridProps {
  blocks: readonly ScheduleBlock[];
  index: ScheduleIndex;
  /** Minutes the grid spans (already widened to its classes). */
  grid: { start: number; end: number };
  /** Writers: an empty slot was clicked (weekday 1–5, minute snapped to 5). */
  onSlot?: (weekday: number, minute: number) => void;
  /** Writers: a class was clicked. Without it, classes are read-only. */
  onOpen?: (block: ScheduleBlock) => void;
}

/**
 * Monday–Friday on a 5-minute grid with labelled hour lines. Classes that
 * overlap in one column (several groups of a grade) sit side by side. On a
 * phone the grid scrolls sideways INSIDE its own box; the page never does.
 */
export function WeekGrid({ blocks, index, grid, onSlot, onOpen }: WeekGridProps): ReactElement {
  const height = px(grid.end - grid.start);
  const hours: number[] = [];
  for (let h = Math.ceil(grid.start / HOUR) * HOUR; h <= grid.end; h += HOUR) hours.push(h);
  const firstLine = px((HOUR - (grid.start % HOUR)) % HOUR);

  return (
    <div role="region" aria-label="Horario de la semana" style={{ ...card, overflowX: "auto", padding: "0 0 12px" }}>
      <div style={{ display: "grid", gridTemplateColumns: `${GUTTER_PX}px repeat(5, minmax(${DAY_MIN_PX}px, 1fr))`, minWidth: GUTTER_PX + 5 * DAY_MIN_PX }}>
        <div />
        {WEEKDAYS.map((d) => (
          <div key={d.weekday} style={{ height: HEADER_PX, display: "grid", placeItems: "center", fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 14, color: "var(--text)", borderBottom: "1px solid var(--border)" }}>
            {d.label}
          </div>
        ))}

        <div aria-hidden style={{ position: "relative", height }}>
          {hours.map((h) => (
            <span key={h} style={{ position: "absolute", top: Math.max(0, px(h - grid.start) - 8), right: 8, fontSize: 12, color: "var(--text-faint)", fontVariantNumeric: "tabular-nums" }}>
              {toHHMM(h)}
            </span>
          ))}
        </div>

        {WEEKDAYS.map((d) => {
          const dayBlocks = blocks.filter((b) => b.weekday === d.weekday);
          const placed = layoutColumns(dayBlocks.map((b) => ({ id: b.id, start: toMinutes(b.startsAt), end: toMinutes(b.endsAt) })));
          return (
            <div key={d.weekday} style={{ position: "relative", height, borderLeft: "1px solid var(--border)", backgroundImage: "linear-gradient(to bottom, var(--border) 0 1px, transparent 1px)", backgroundSize: `100% ${px(HOUR)}px`, backgroundPosition: `0 ${firstLine}px` }}>
              {onSlot !== undefined && <SlotButtons weekday={d.weekday} label={d.label} grid={grid} hours={hours} onSlot={onSlot} />}
              {dayBlocks.map((b) => (
                <ClassBlock key={b.id} block={b} index={index} grid={grid} place={placed.get(b.id) ?? { column: 0, columns: 1 }} onOpen={onOpen} />
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}

/**
 * One transparent button per hour of the day; where the click lands inside it
 * picks the 5-minute row. Out of the tab order: the "Agregar clase" button
 * does the same from the keyboard, without 40 stops before the classes.
 */
function SlotButtons({ weekday, label, grid, hours, onSlot }: { weekday: number; label: string; grid: { start: number; end: number }; hours: number[]; onSlot: (weekday: number, minute: number) => void }): ReactElement {
  const starts = [grid.start, ...hours.filter((h) => h > grid.start && h < grid.end)];
  const day = label.toLocaleLowerCase("es-MX");
  return (
    <>
      {starts.map((from, i) => {
        const to = starts[i + 1] ?? grid.end;
        const click = (e: MouseEvent<HTMLButtonElement>): void => {
          const offset = e.clientY - e.currentTarget.getBoundingClientRect().top;
          const minute = Math.min(snapToSlot(from + (Math.max(0, offset) / ROW_PX) * SLOT_MINUTES), grid.end - SLOT_MINUTES);
          onSlot(weekday, Math.max(from, minute));
        };
        return (
          <button key={from} type="button" tabIndex={-1} aria-label={`Agregar clase el ${day} a las ${toHHMM(from)}`} onClick={click}
            style={{ position: "absolute", left: 0, right: 0, top: px(from - grid.start), height: px(to - from), background: "transparent", cursor: "copy" }} />
        );
      })}
    </>
  );
}

function ClassBlock({ block, index, grid, place, onOpen }: { block: ScheduleBlock; index: ScheduleIndex; grid: { start: number }; place: { column: number; columns: number }; onOpen?: (block: ScheduleBlock) => void }): ReactElement {
  const start = toMinutes(block.startsAt);
  const end = toMinutes(block.endsAt);
  const subject = index.subjectName(block.subjectId);
  const color = COLOR[subjectColor(subject)].v;
  const style: CSSProperties = {
    position: "absolute",
    top: px(start - grid.start) + 1,
    height: Math.max(px(end - start) - 2, MIN_BLOCK_PX),
    left: `calc(${(place.column / place.columns) * 100}% + 3px)`,
    width: `calc(${100 / place.columns}% - 6px)`,
    padding: "4px 7px",
    borderRadius: 10,
    borderLeft: `4px solid ${color}`,
    background: `color-mix(in oklab, ${color} 16%, var(--surface))`,
    color: "var(--text)",
    textAlign: "left",
    overflow: "hidden",
    fontSize: 12,
    lineHeight: 1.3,
    boxShadow: "var(--shadow-card)",
  };
  const body = (
    <>
      <strong style={{ display: "block", fontSize: 12.5, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{subject}</strong>
      <span style={{ display: "block", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{index.groupLabel(block.groupId)} · {index.teacherName(block.teacherId)}</span>
      <span style={{ display: "block", color: "var(--text-muted)", fontVariantNumeric: "tabular-nums" }}>{block.startsAt}–{block.endsAt}</span>
    </>
  );
  const label = blockLabel(index, block);
  if (onOpen === undefined) {
    return <div data-testid="schedule-block" role="note" aria-label={label} style={style}>{body}</div>;
  }
  return (
    <button type="button" data-testid="schedule-block" aria-label={label} onClick={() => onOpen(block)} style={{ ...style, cursor: "pointer", border: "none", borderLeft: style.borderLeft }}>
      {body}
    </button>
  );
}
