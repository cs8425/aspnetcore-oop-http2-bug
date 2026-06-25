<%@ WebHandler Language = "C#" Class="proxy" %>

using System;
using System.IO;
using System.Web;


public class proxy : IHttpHandler
{
	public void ProcessRequest(HttpContext context)
	{
		context.Response.ContentType = "text/plain; charset=utf-8";
		context.Response.Write(string.Format("SERVER_PROTOCOL={0}\n", context.Request.ServerVariables["SERVER_PROTOCOL"]));
		context.Response.Write(string.Format("REQUEST_METHOD={0}\n", context.Request.ServerVariables["REQUEST_METHOD"]));
		context.Response.Write(string.Format("REMOTE_ADDR={0}\n", context.Request.ServerVariables["REMOTE_ADDR"]));
		context.Response.Write("=================================\n");
		context.Response.Write(string.Format("context.Request.ApplicationPath={0}\n", context.Request.ApplicationPath));
		context.Response.Write(string.Format("context.Request.CurrentExecutionFilePath={0}\n", context.Request.CurrentExecutionFilePath));
		// context.Response.Write(string.Format("context.Request.context.Request.PhysicalApplicationPath={0}\n", context.Request.PhysicalApplicationPath));
		context.Response.Write("=================================\n");
		foreach (var headerKey in context.Request.Headers.AllKeys)
		{
			string headerValue = context.Request.Headers[headerKey];
			context.Response.Write(headerKey + ": " + headerValue + "\n");
		}
		context.Response.Write("=================================\n");
	}

	public bool IsReusable
	{
		get { return true; }
	}
}
