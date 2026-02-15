<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkSuspendClient extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest1 $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest1 $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/sdk/clients/{id}/suspend');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\StatusChangeRequest1) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\StatusResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\StatusResponse', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkSuspendClientNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkSuspendClientUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkSuspendClientForbiddenException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkSuspendClientBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}