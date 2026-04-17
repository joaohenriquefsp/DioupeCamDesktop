export namespace domain {
	
	export class Config {
	    ip: string;
	    port: number;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}

}

