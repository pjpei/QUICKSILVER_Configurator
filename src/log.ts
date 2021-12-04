export enum LogLevel {
  Debug = "debug",
  Info = "info",
  Warning = "warning",
  Error = "error",
}

export class Log {

  public static level = LogLevel.Debug;

  public static debug(prefix: string, ...data: any[]) {
    Log.log(LogLevel.Debug, prefix, ...data);
  }

  public static info(prefix: string, ...data: any[]) {
    Log.log(LogLevel.Info, prefix, ...data);
  }

  public static warn(prefix: string, ...data: any[]) {
    Log.log(LogLevel.Warning, prefix, ...data);
  }

  public static warning(prefix: string, ...data: any[]) {
    Log.log(LogLevel.Warning, prefix, ...data);
  }

  public static error(prefix: string, ...data: any[]) {
    Log.log(LogLevel.Error, prefix, ...data);
  }

  public static log(level: LogLevel, prefix?: string, ...data: any[]) {
    let str = "[" + level + "]";
    if (prefix && prefix.length) {
      str += "[" + prefix + "]";
    }
    switch (level) {
      case LogLevel.Debug:
      case LogLevel.Info:
      default:
        console.log(str, ...data);
        break;
      case LogLevel.Warning:
        console.warn(str, ...data);
        break;
      case LogLevel.Error:
        console.error(str, ...data);
        break;
    }
  }
}