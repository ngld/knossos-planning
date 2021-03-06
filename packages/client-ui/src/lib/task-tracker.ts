import { useState, useEffect } from 'react';
import { makeObservable, action, observable, computed } from 'mobx';
import EventEmitter from 'eventemitter3';
import { LogMessage, LogMessage_LogLevel, ClientSentEvent } from '@api/client';

export interface TaskState {
  id: number;
  label: string;
  progress: number;
  status: string;
  error: boolean;
  indeterminate: boolean;
  started: number;
  logMessages: LogMessage[];
  logContainer: HTMLDivElement,
}

export const logLevelMap: Record<LogMessage_LogLevel, string> = {} as Record<
  LogMessage_LogLevel,
  string
>;
for (const [name, level] of Object.entries(LogMessage_LogLevel)) {
  logLevelMap[level as LogMessage_LogLevel] = name;
}

function getLogTime(task: TaskState, line: LogMessage): string {
  const time = line.time;
  if (!time) {
    return '00:00';
  }

  const duration = time.seconds - task.started;
  const minutes = Math.floor(duration / 60);
  const seconds = duration % 60;

  let result = (minutes < 10 ? '0' : '') + String(minutes) + ':';
  result += (seconds < 10 ? '0' : '') + String(seconds);
  return result;
}

export class TaskTracker extends EventEmitter {
  _idCounter: number;
  tasks: TaskState[];
  taskMap: Record<string, TaskState>;

  constructor() {
    super();
    this._idCounter = 1;
    this.tasks = [];
    this.taskMap = {};

    makeObservable(this, {
      tasks: observable,
      taskMap: observable,
      active: computed,
      listen: action,
      startTask: action,
      updateTask: action,
      removeTask: action,
    });
  }

  get active(): number {
    let count = 0;
    for (const task of this.tasks) {
      if (task.progress < 1 && !task.error) {
        count++;
      }
    }
    return count;
  }

  listen(): () => void {
    const listener = action((queue: ArrayBuffer[]) => {
      if (!Array.isArray(queue)) {
        console.error('Invalid queue passed to listener()!');
      }
      try {
        for (const msg of queue) {
          const ev = ClientSentEvent.fromBinary(new Uint8Array(msg));
          this.updateTask(ev);
        }
      } catch (e) {
        console.error(e);
      }
    });

    knAddMessageListener(listener);
    return () => knRemoveMessageListener(listener);
  }

  startTask(label: string): number {
    const id = this._idCounter++;
    const task = {
      id,
      label,
      progress: 0,
      status: 'Initialising',
      error: false,
      indeterminate: true,
      started: Math.floor(Date.now() / 1000),
      logMessages: [],
      logContainer: document.createElement('div'),
    } as TaskState;

    this.taskMap[id] = task;
    this.tasks.unshift(this.taskMap[id]);
    this.emit('new', id);

    return id;
  }

  updateTask(ev: ClientSentEvent): void {
    const task = this.taskMap[ev.ref];
    if (!task) {
      console.error(`Got update for missing task ${ev.ref}`);
      return;
    }

    switch (ev.payload.oneofKind) {
      case 'message':
        // task.logMessages.push(ev.payload.message);
        const msg = ev.payload.message;
        const line = document.createElement('div');
        line.setAttribute('title', msg.sender);
        line.setAttribute('class', 'log-' + (logLevelMap[msg.level] ?? 'info').toLowerCase());

        const lineText = document.createElement('span');
        lineText.setAttribute('class', 'font-mono');
        lineText.innerText = `[${getLogTime(task, msg)}]:`;

        line.appendChild(lineText);
        line.appendChild(document.createTextNode(' ' + msg.message));

        line.innerHTML = line.innerHTML.replace(/\n/g, '<br>');
        task.logContainer.appendChild(line);
        break;
      case 'progress':
        {
          const info = ev.payload.progress;
          if (info.progress >= 0) {
            task.progress = info.progress;
          }
          if (info.description !== '') {
            task.status = info.description;
          }
          task.error = info.error;
          task.indeterminate = info.indeterminate;
        }
        break;
      case 'result':
        {
          const taskResult = ev.payload.result;
          task.indeterminate = false;

          if (!taskResult.success) {
            task.error = true;
          } else {
            task.progress = 1;
          }
        }
        break;
    }

    this.taskMap[ev.ref] = task;
  }

  removeTask(id: number): void {
    let taskIdx = -1;
    for (let i = 0; i < this.tasks.length; i++) {
      if (this.tasks[i].id === id) {
        taskIdx = i;
        break;
      }
    }

    if (taskIdx === -1) {
      console.error(`Task with id ${id} not found in the current task list.`);
      return;
    }

    this.tasks.splice(taskIdx, 1);
    delete this.taskMap[id];
  }
}

export function useTaskTracker(): TaskTracker {
  const [tracker] = useState(() => new TaskTracker());

  useEffect(() => {
    return tracker.listen();
  }, [tracker]);

  return tracker;
}
